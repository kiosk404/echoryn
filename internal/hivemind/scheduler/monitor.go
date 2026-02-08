package scheduler

import (
	"context"
	"sync"
	"time"
)

// --------------------------------------------------------------------------
// Monitor interface
// --------------------------------------------------------------------------

// Monitor watches running tasks and enforces timeouts, detects stalls,
// and triggers re-scheduling when a Golem node becomes unresponsive.
type Monitor interface {
	// Watch begins monitoring a task's execution on the assigned node.
	Watch(ctx context.Context, task *protocol.Task) error

	// Unwatch stops monitoring a task (called when task completes or is cancelled).
	Unwatch(taskID string)

	// RecordHeartbeat records a heartbeat from a running task, resetting its stall timer.
	RecordHeartbeat(taskID string)

	// ActiveTasks returns the set of currently monitored task IDs.
	ActiveTasks() []string

	// Start begins the monitor's background polling loop.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the monitor.
	Stop(ctx context.Context) error
}

// MonitorEventHandler receives notifications when the monitor detects
// timeout or stall conditions.
type MonitorEventHandler interface {
	// OnTaskTimeout is called when a task exceeds its configured timeout.
	OnTaskTimeout(ctx context.Context, taskID string)

	// OnTaskStalled is called when no heartbeat has been received within
	// the stall detection window.
	OnTaskStalled(ctx context.Context, taskID string)
}

// --------------------------------------------------------------------------
// MonitorConfig
// --------------------------------------------------------------------------

// MonitorConfig holds configuration for the task execution monitor.
type MonitorConfig struct {
	// PollInterval is the interval at which the monitor checks for timeouts and stalls.
	PollInterval time.Duration

	// StallThreshold is the maximum duration without a heartbeat before a task
	// is considered stalled.
	StallThreshold time.Duration

	// DefaultTimeout is applied to tasks that do not specify their own timeout.
	DefaultTimeout time.Duration
}

// DefaultMonitorConfig returns a MonitorConfig with sensible defaults.
func DefaultMonitorConfig() MonitorConfig {
	return MonitorConfig{
		PollInterval:   10 * time.Second,
		StallThreshold: 60 * time.Second,
		DefaultTimeout: 5 * time.Minute,
	}
}

// --------------------------------------------------------------------------
// taskMonitor — implementation
// --------------------------------------------------------------------------

// taskMonitor is the concrete Monitor implementation.
type taskMonitor struct {
	config  MonitorConfig
	handler MonitorEventHandler

	mu       sync.RWMutex
	watched  map[string]*watchedTask
	stopCh   chan struct{}
	stopOnce sync.Once
}

type watchedTask struct {
	task          *protocol.Task
	startedAt     time.Time
	lastHeartbeat time.Time
	timeout       time.Duration
}

// NewMonitor creates a new task execution monitor.
func NewMonitor(config MonitorConfig, handler MonitorEventHandler) Monitor {
	return &taskMonitor{
		config:  config,
		handler: handler,
		watched: make(map[string]*watchedTask),
		stopCh:  make(chan struct{}),
	}
}

// Watch begins monitoring a task.
func (m *taskMonitor) Watch(_ context.Context, task *protocol.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	timeout := task.Timeout
	if timeout == 0 {
		timeout = m.config.DefaultTimeout
	}

	now := time.Now()
	m.watched[task.ID] = &watchedTask{
		task:          task,
		startedAt:     now,
		lastHeartbeat: now,
		timeout:       timeout,
	}
	return nil
}

// Unwatch stops monitoring a task.
func (m *taskMonitor) Unwatch(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.watched, taskID)
}

// RecordHeartbeat resets the stall timer for a task.
func (m *taskMonitor) RecordHeartbeat(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if wt, ok := m.watched[taskID]; ok {
		wt.lastHeartbeat = time.Now()
	}
}

// ActiveTasks returns the IDs of all currently monitored tasks.
func (m *taskMonitor) ActiveTasks() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.watched))
	for id := range m.watched {
		ids = append(ids, id)
	}
	return ids
}

// Start begins the background polling loop.
func (m *taskMonitor) Start(ctx context.Context) error {
	go m.pollLoop(ctx)
	return nil
}

// Stop gracefully shuts down the monitor.
func (m *taskMonitor) Stop(_ context.Context) error {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
	return nil
}

// pollLoop periodically checks all watched tasks for timeout and stall conditions.
func (m *taskMonitor) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkTasks(ctx)
		}
	}
}

// checkTasks inspects all watched tasks and fires events for timeouts and stalls.
func (m *taskMonitor) checkTasks(ctx context.Context) {
	m.mu.RLock()
	// Snapshot the task IDs to avoid holding the lock during handler calls.
	type check struct {
		id      string
		timeout bool
		stalled bool
	}
	var checks []check
	now := time.Now()
	for id, wt := range m.watched {
		c := check{id: id}
		if now.Sub(wt.startedAt) > wt.timeout {
			c.timeout = true
		}
		if now.Sub(wt.lastHeartbeat) > m.config.StallThreshold {
			c.stalled = true
		}
		if c.timeout || c.stalled {
			checks = append(checks, c)
		}
	}
	m.mu.RUnlock()

	// Fire events outside the lock.
	for _, c := range checks {
		if c.timeout {
			m.handler.OnTaskTimeout(ctx, c.id)
			// Remove timed-out tasks from watch list.
			m.Unwatch(c.id)
		} else if c.stalled {
			m.handler.OnTaskStalled(ctx, c.id)
		}
	}
}

// --------------------------------------------------------------------------
// SchedulerStats — aggregate statistics
// --------------------------------------------------------------------------

// SchedulerStats contains aggregate statistics about the scheduler's operation.
type SchedulerStats struct {
	// TotalSubmitted is the total number of tasks ever submitted.
	TotalSubmitted int64

	// TotalCompleted is the total number of tasks that completed successfully.
	TotalCompleted int64

	// TotalFailed is the total number of tasks that failed.
	TotalFailed int64

	// TotalCancelled is the total number of tasks that were cancelled.
	TotalCancelled int64

	// TotalTimedOut is the total number of tasks that timed out.
	TotalTimedOut int64

	// CurrentQueued is the number of tasks currently in the queue.
	CurrentQueued int

	// CurrentRunning is the number of tasks currently being executed.
	CurrentRunning int

	// AverageLatency is the average time from submission to assignment.
	AverageLatency time.Duration

	// AverageExecutionTime is the average time from assignment to completion.
	AverageExecutionTime time.Duration

	// NodeStats maps node IDs to per-node scheduling statistics.
	NodeStats map[string]*NodeSchedulerStats

	// CollectedAt records when these statistics were gathered.
	CollectedAt time.Time
}

// NodeSchedulerStats contains per-node scheduling statistics.
type NodeSchedulerStats struct {
	// NodeID is the Golem node identifier.
	NodeID string

	// TasksAssigned is the total number of tasks assigned to this node.
	TasksAssigned int64

	// TasksCompleted is the total number of tasks this node completed successfully.
	TasksCompleted int64

	// TasksFailed is the total number of tasks this node failed.
	TasksFailed int64

	// AverageExecutionTime is the average task execution time on this node.
	AverageExecutionTime time.Duration

	// LastAssignedAt records when a task was last assigned to this node.
	LastAssignedAt time.Time
}

// StatsCollector tracks and aggregates scheduler statistics.
type StatsCollector struct {
	mu    sync.Mutex
	stats SchedulerStats

	// Track running tasks for CurrentRunning count.
	running map[string]time.Time // taskID -> assignedAt

	// Track latency samples for averaging.
	latencySamples   []time.Duration
	executionSamples []time.Duration
	maxSampleCount   int
}

// NewStatsCollector creates a new statistics collector.
func NewStatsCollector() *StatsCollector {
	return &StatsCollector{
		running:        make(map[string]time.Time),
		maxSampleCount: 1000,
		stats: SchedulerStats{
			NodeStats: make(map[string]*NodeSchedulerStats),
		},
	}
}

// RecordSubmission records a task submission.
func (c *StatsCollector) RecordSubmission() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats.TotalSubmitted++
}

// RecordAssignment records a task assignment to a node.
func (c *StatsCollector) RecordAssignment(taskID, nodeID string, latency time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.running[taskID] = time.Now()
	c.latencySamples = append(c.latencySamples, latency)
	if len(c.latencySamples) > c.maxSampleCount {
		c.latencySamples = c.latencySamples[1:]
	}

	ns := c.getOrCreateNodeStats(nodeID)
	ns.TasksAssigned++
	ns.LastAssignedAt = time.Now()
}

// RecordCompletion records a task completion.
func (c *StatsCollector) RecordCompletion(taskID, nodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stats.TotalCompleted++

	if assignedAt, ok := c.running[taskID]; ok {
		execTime := time.Since(assignedAt)
		c.executionSamples = append(c.executionSamples, execTime)
		if len(c.executionSamples) > c.maxSampleCount {
			c.executionSamples = c.executionSamples[1:]
		}
		delete(c.running, taskID)

		if nodeID != "" {
			ns := c.getOrCreateNodeStats(nodeID)
			ns.TasksCompleted++
		}
	}
}

// RecordFailure records a task failure.
func (c *StatsCollector) RecordFailure(taskID, nodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stats.TotalFailed++
	delete(c.running, taskID)

	if nodeID != "" {
		ns := c.getOrCreateNodeStats(nodeID)
		ns.TasksFailed++
	}
}

// RecordCancellation records a task cancellation.
func (c *StatsCollector) RecordCancellation(taskID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stats.TotalCancelled++
	delete(c.running, taskID)
}

// RecordTimeout records a task timeout.
func (c *StatsCollector) RecordTimeout(taskID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stats.TotalTimedOut++
	delete(c.running, taskID)
}

// Snapshot returns a copy of the current statistics.
func (c *StatsCollector) Snapshot(queueLen int) SchedulerStats {
	c.mu.Lock()
	defer c.mu.Unlock()

	snap := c.stats
	snap.CurrentQueued = queueLen
	snap.CurrentRunning = len(c.running)
	snap.AverageLatency = averageDuration(c.latencySamples)
	snap.AverageExecutionTime = averageDuration(c.executionSamples)
	snap.CollectedAt = time.Now()

	// Deep-copy NodeStats.
	snap.NodeStats = make(map[string]*NodeSchedulerStats, len(c.stats.NodeStats))
	for k, v := range c.stats.NodeStats {
		copied := *v
		snap.NodeStats[k] = &copied
	}

	return snap
}

func (c *StatsCollector) getOrCreateNodeStats(nodeID string) *NodeSchedulerStats {
	if ns, ok := c.stats.NodeStats[nodeID]; ok {
		return ns
	}
	ns := &NodeSchedulerStats{NodeID: nodeID}
	c.stats.NodeStats[nodeID] = ns
	return ns
}

func averageDuration(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	var total time.Duration
	for _, s := range samples {
		total += s
	}
	return total / time.Duration(len(samples))
}
