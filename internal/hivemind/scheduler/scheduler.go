package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Scheduler interface {
	// Schedule enqueues a scheduling request. The request's Mode field determines
	// whether a specific Golem is targeted (DirectMode) or the AI selector
	// picks the best one (AIMode).
	Schedule(ctx context.Context, req *ScheduleRequest) (*ScheduleDecision, error)

	// Cancel aborts a pending or running task by ID.
	Cancel(ctx context.Context, taskID string) error

	// Status returns the current state of a task.
	Status(ctx context.Context, taskID string) (*protocol.Task, error)

	// Stats returns aggregate scheduler statistics.
	Stats() SchedulerStats

	// Subscribe registers a listener for task lifecycle events.
	Subscribe(listener TaskEventListener)

	// Unsubscribe removes a previously registered listener.
	Unsubscribe(listener TaskEventListener)

	// Start begins the scheduler's background processing loops.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the scheduler.
	Stop(ctx context.Context) error
}

// --------------------------------------------------------------------------
// TaskDispatcher — abstraction for actually sending tasks to Golem nodes
// --------------------------------------------------------------------------

// TaskDispatcher sends a task to a specific Golem node for execution.
// This abstracts the transport layer (gRPC, WebSocket, etc.) from the
// scheduler's decision logic.
type TaskDispatcher interface {
	// Dispatch sends a task to the specified node.
	Dispatch(ctx context.Context, nodeID string, task *protocol.Task) error
}

// --------------------------------------------------------------------------
// SchedulerConfig — Options pattern (k8s style)
// --------------------------------------------------------------------------

// SchedulerConfig holds all configuration for the scheduler.
type SchedulerConfig struct {
	// DispatchConcurrency is the maximum number of concurrent dispatch operations.
	DispatchConcurrency int

	// ScheduleLoopInterval is the interval at which the scheduler polls the queue
	// for pending requests.
	ScheduleLoopInterval time.Duration

	// MaxRetries is the maximum number of times a task can be rescheduled after failure.
	MaxRetries int

	// DefaultScoringWeights are the weights used by the AI selector when none
	// are specified in the request.
	DefaultScoringWeights ScoringWeights

	// MonitorConfig configures the task execution monitor.
	MonitorConfig MonitorConfig
}

// DefaultSchedulerConfig returns a SchedulerConfig with sensible defaults.
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		DispatchConcurrency:   8,
		ScheduleLoopInterval:  500 * time.Millisecond,
		MaxRetries:            3,
		DefaultScoringWeights: DefaultScoringWeights(),
		MonitorConfig:         DefaultMonitorConfig(),
	}
}

// CompletedSchedulerConfig is a sealed configuration ready for use.
// Follows the k8s pattern: Config → CompletedConfig → New().
type CompletedSchedulerConfig struct {
	config     SchedulerConfig
	provider   ProfileProvider
	dispatcher TaskDispatcher
}

// Complete validates the configuration and seals it.
func (c SchedulerConfig) Complete(provider ProfileProvider, dispatcher TaskDispatcher) (*CompletedSchedulerConfig, error) {
	if provider == nil {
		return nil, fmt.Errorf("scheduler: ProfileProvider must not be nil")
	}
	if dispatcher == nil {
		return nil, fmt.Errorf("scheduler: TaskDispatcher must not be nil")
	}
	if c.DispatchConcurrency <= 0 {
		c.DispatchConcurrency = 8
	}
	if c.ScheduleLoopInterval <= 0 {
		c.ScheduleLoopInterval = 500 * time.Millisecond
	}
	return &CompletedSchedulerConfig{
		config:     c,
		provider:   provider,
		dispatcher: dispatcher,
	}, nil
}

// New creates a fully initialised Scheduler from the completed configuration.
func (cc *CompletedSchedulerConfig) New() Scheduler {
	stats := NewStatsCollector()

	// Build selectors.
	directSel := NewDirectSelector(cc.provider)
	aiSel := NewDefaultAISelector()

	// Build monitor with the scheduler as event handler.
	s := &defaultScheduler{
		config:     cc.config,
		provider:   cc.provider,
		dispatcher: cc.dispatcher,
		queue:      NewPriorityQueue(),
		directSel:  directSel,
		aiSel:      aiSel,
		stats:      stats,
		tasks:      make(map[string]*taskRecord),
		stopCh:     make(chan struct{}),
	}

	s.monitor = NewMonitor(cc.config.MonitorConfig, s)

	return s
}

// --------------------------------------------------------------------------
// defaultScheduler — Facade implementation
// --------------------------------------------------------------------------

type taskRecord struct {
	task     *protocol.Task
	decision *ScheduleDecision
	request  *ScheduleRequest
	retries  int
}

type defaultScheduler struct {
	config     SchedulerConfig
	provider   ProfileProvider
	dispatcher TaskDispatcher
	queue      Queue
	directSel  NodeSelector
	aiSel      NodeSelector
	monitor    Monitor
	stats      *StatsCollector

	mu        sync.RWMutex
	tasks     map[string]*taskRecord
	listeners []TaskEventListener

	stopCh   chan struct{}
	stopOnce sync.Once
}

// Schedule enqueues a scheduling request and attempts immediate dispatch.
func (s *defaultScheduler) Schedule(ctx context.Context, req *ScheduleRequest) (*ScheduleDecision, error) {
	if req.Task == nil {
		return nil, fmt.Errorf("scheduler: task must not be nil")
	}

	// Record submission.
	s.stats.RecordSubmission()

	// Try immediate dispatch.
	decision, err := s.tryDispatch(ctx, req)
	if err == nil {
		return decision, nil
	}

	// If immediate dispatch fails, enqueue for background processing.
	if enqErr := s.queue.Enqueue(req); enqErr != nil {
		return nil, fmt.Errorf("scheduler: failed to enqueue task %q: %w", req.Task.ID, enqErr)
	}

	s.emitEvent(&TaskEvent{
		Type:      EventTypeSubmitted,
		Task:      req.Task,
		Timestamp: time.Now(),
	})

	return nil, fmt.Errorf("scheduler: immediate dispatch failed (%w), task %q queued for retry", err, req.Task.ID)
}

// Cancel aborts a pending or running task.
func (s *defaultScheduler) Cancel(ctx context.Context, taskID string) error {
	// Try to remove from queue first.
	if s.queue.Remove(taskID) {
		s.stats.RecordCancellation(taskID)
		s.mu.Lock()
		if rec, ok := s.tasks[taskID]; ok {
			rec.task.Status = protocol.TaskStatusCancelled
		}
		s.mu.Unlock()

		s.emitEvent(&TaskEvent{
			Type:      EventTypeCancelled,
			Task:      s.getTask(taskID),
			Timestamp: time.Now(),
		})
		return nil
	}

	// Task might be running — unwatch it.
	s.monitor.Unwatch(taskID)
	s.stats.RecordCancellation(taskID)

	s.mu.Lock()
	if rec, ok := s.tasks[taskID]; ok {
		rec.task.Status = protocol.TaskStatusCancelled
	}
	s.mu.Unlock()

	s.emitEvent(&TaskEvent{
		Type:      EventTypeCancelled,
		Task:      s.getTask(taskID),
		Timestamp: time.Now(),
	})
	return nil
}

// Status returns the current state of a task.
func (s *defaultScheduler) Status(_ context.Context, taskID string) (*protocol.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("scheduler: task %q not found", taskID)
	}
	return rec.task, nil
}

// Stats returns aggregate scheduler statistics.
func (s *defaultScheduler) Stats() SchedulerStats {
	return s.stats.Snapshot(s.queue.Len())
}

// Subscribe registers a listener for task lifecycle events.
func (s *defaultScheduler) Subscribe(listener TaskEventListener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, listener)
}

// Unsubscribe removes a previously registered listener.
func (s *defaultScheduler) Unsubscribe(listener TaskEventListener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, l := range s.listeners {
		if l == listener {
			s.listeners = append(s.listeners[:i], s.listeners[i+1:]...)
			return
		}
	}
}

// Start begins the scheduler's background processing loops.
func (s *defaultScheduler) Start(ctx context.Context) error {
	if err := s.monitor.Start(ctx); err != nil {
		return fmt.Errorf("scheduler: failed to start monitor: %w", err)
	}
	go s.scheduleLoop(ctx)
	return nil
}

// Stop gracefully shuts down the scheduler.
func (s *defaultScheduler) Stop(ctx context.Context) error {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	return s.monitor.Stop(ctx)
}

// --------------------------------------------------------------------------
// MonitorEventHandler implementation — Observer pattern
// --------------------------------------------------------------------------

// OnTaskTimeout handles task timeout events from the monitor.
func (s *defaultScheduler) OnTaskTimeout(_ context.Context, taskID string) {
	s.stats.RecordTimeout(taskID)

	s.mu.Lock()
	if rec, ok := s.tasks[taskID]; ok {
		rec.task.Status = protocol.TaskStatusTimedOut
	}
	s.mu.Unlock()

	s.emitEvent(&TaskEvent{
		Type:      EventTypeTimedOut,
		Task:      s.getTask(taskID),
		Timestamp: time.Now(),
	})
}

// OnTaskStalled handles task stall events from the monitor.
func (s *defaultScheduler) OnTaskStalled(ctx context.Context, taskID string) {
	s.mu.Lock()
	rec, ok := s.tasks[taskID]
	if !ok {
		s.mu.Unlock()
		return
	}

	// Attempt rescheduling if retries remain.
	if rec.retries < s.config.MaxRetries && rec.request != nil {
		rec.retries++
		req := rec.request
		s.mu.Unlock()

		// Re-enqueue.
		_ = s.queue.Enqueue(req)

		s.emitEvent(&TaskEvent{
			Type:      EventTypeRescheduled,
			Task:      rec.task,
			Timestamp: time.Now(),
		})
		return
	}
	s.mu.Unlock()

	// Max retries exceeded — mark as failed.
	s.stats.RecordFailure(taskID, "")
	s.emitEvent(&TaskEvent{
		Type:      EventTypeFailed,
		Task:      s.getTask(taskID),
		Error:     fmt.Errorf("task stalled after %d retries", s.config.MaxRetries),
		Timestamp: time.Now(),
	})
}

// --------------------------------------------------------------------------
// Internal scheduling logic
// --------------------------------------------------------------------------

// tryDispatch attempts to immediately select a node and dispatch the task.
func (s *defaultScheduler) tryDispatch(ctx context.Context, req *ScheduleRequest) (*ScheduleDecision, error) {
	// Gather candidate profiles.
	candidates, err := s.provider.ListProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list Golem profiles: %w", err)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no Golem nodes available")
	}

	// Choose selector based on mode.
	var selector NodeSelector
	switch req.Mode {
	case DirectMode:
		selector = s.directSel
	case AIMode:
		selector = s.aiSel
	default:
		return nil, fmt.Errorf("unknown schedule mode %q", req.Mode)
	}

	// Select the best node.
	decision, err := selector.Select(ctx, req, candidates)
	if err != nil {
		return nil, err
	}

	// Assign the task to the selected node.
	req.Task.AssignedNodeID = decision.SelectedNodeID
	req.Task.Status = protocol.TaskStatusAssigned
	now := time.Now()
	req.Task.StartedAt = &now
	decision.RequestID = req.Task.ID

	// Record in task map.
	s.mu.Lock()
	s.tasks[req.Task.ID] = &taskRecord{
		task:     req.Task,
		decision: decision,
		request:  req,
	}
	s.mu.Unlock()

	// Dispatch to the Golem node.
	if err := s.dispatcher.Dispatch(ctx, decision.SelectedNodeID, req.Task); err != nil {
		return nil, fmt.Errorf("failed to dispatch task %q to node %q: %w", req.Task.ID, decision.SelectedNodeID, err)
	}

	// Record assignment stats.
	s.stats.RecordAssignment(req.Task.ID, decision.SelectedNodeID, decision.Latency)

	// Start monitoring.
	_ = s.monitor.Watch(ctx, req.Task)

	// Emit event.
	s.emitEvent(&TaskEvent{
		Type:      EventTypeAssigned,
		Task:      req.Task,
		Decision:  decision,
		NodeID:    decision.SelectedNodeID,
		Timestamp: time.Now(),
	})

	return decision, nil
}

// scheduleLoop is the background goroutine that processes the queue.
func (s *defaultScheduler) scheduleLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.ScheduleLoopInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.processQueue(ctx)
		}
	}
}

// processQueue attempts to dispatch all pending requests in the queue.
func (s *defaultScheduler) processQueue(ctx context.Context) {
	for {
		req := s.queue.Peek()
		if req == nil {
			return
		}

		_, err := s.tryDispatch(ctx, req)
		if err != nil {
			// Cannot dispatch right now — leave in queue and retry later.
			return
		}

		// Successfully dispatched — remove from queue.
		s.queue.Dequeue()
	}
}

// --------------------------------------------------------------------------
// Event emission
// --------------------------------------------------------------------------

func (s *defaultScheduler) emitEvent(event *TaskEvent) {
	s.mu.RLock()
	listeners := make([]TaskEventListener, len(s.listeners))
	copy(listeners, s.listeners)
	s.mu.RUnlock()

	for _, l := range listeners {
		l.OnEvent(event)
	}
}

func (s *defaultScheduler) getTask(taskID string) *protocol.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if rec, ok := s.tasks[taskID]; ok {
		return rec.task
	}
	return nil
}

// --------------------------------------------------------------------------
// ReportResult / ReportProgress — called by the transport layer
// --------------------------------------------------------------------------

// ReportProgress records incremental progress from a running task.
func (s *defaultScheduler) ReportProgress(ctx context.Context, progress *protocol.TaskProgress) {
	s.monitor.RecordHeartbeat(progress.TaskID)

	s.emitEvent(&TaskEvent{
		Type:      EventTypeProgress,
		Task:      s.getTask(progress.TaskID),
		Progress:  progress,
		Timestamp: time.Now(),
	})
}

// ReportResult records the final result of a completed task.
func (s *defaultScheduler) ReportResult(_ context.Context, result *protocol.TaskResult) {
	s.monitor.Unwatch(result.TaskID)

	s.mu.Lock()
	rec, ok := s.tasks[result.TaskID]
	if ok {
		now := time.Now()
		rec.task.CompletedAt = &now
		if result.Success {
			rec.task.Status = protocol.TaskStatusCompleted
		} else {
			rec.task.Status = protocol.TaskStatusFailed
		}
	}
	nodeID := ""
	if ok && rec.decision != nil {
		nodeID = rec.decision.SelectedNodeID
	}
	s.mu.Unlock()

	if result.Success {
		s.stats.RecordCompletion(result.TaskID, nodeID)
		s.emitEvent(&TaskEvent{
			Type:      EventTypeCompleted,
			Task:      s.getTask(result.TaskID),
			Result:    result,
			NodeID:    nodeID,
			Timestamp: time.Now(),
		})
	} else {
		s.stats.RecordFailure(result.TaskID, nodeID)
		s.emitEvent(&TaskEvent{
			Type:      EventTypeFailed,
			Task:      s.getTask(result.TaskID),
			Result:    result,
			NodeID:    nodeID,
			Error:     fmt.Errorf("%s", result.Error),
			Timestamp: time.Now(),
		})
	}
}
