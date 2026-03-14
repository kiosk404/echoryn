package observer

import (
	"sync"
	"time"

	"github.com/kiosk404/echoryn/pkg/logger"
)

// executionObserver is the concrete implementation of the Observer interface.
//
// Architecture:
//
//	Emit() → eventCh (buffered) → processLoop goroutine
//	                                  ├── MetricsAggregator.Record()
//	                                  ├── recentEvents ring buffer
//	                                  └── Reporter.evaluate() (periodic)
//
// Design:
//   - Emit() is always non-blocking: if the channel is full, the event is dropped
//     with a logged warning (observer must never slow down execution).
//   - The processLoop runs in a single goroutine, so MetricsAggregator access
//     from the loop is single-writer (though the aggregator is still thread-safe
//     for Snapshot reads from other goroutines).
type executionObserver struct {
	config  Config
	metrics *MetricsAggregator

	// Event ingestion
	eventCh chan Event

	// Recent events ring buffer (for reports)
	recentMu     sync.RWMutex
	recentEvents []Event
	recentIdx    int
	recentFull   bool

	// Health evaluation
	reporter *Reporter

	// Lifecycle
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// New creates a new Observer with the given configuration.
// Call Start() to begin processing events.
func New(config Config) Observer {
	if config.EventBufferSize <= 0 {
		config.EventBufferSize = DefaultConfig().EventBufferSize
	}
	if config.RecentEventsCapacity <= 0 {
		config.RecentEventsCapacity = DefaultConfig().RecentEventsCapacity
	}
	if config.MetricsWindowSize <= 0 {
		config.MetricsWindowSize = DefaultConfig().MetricsWindowSize
	}
	if config.HealthCheckInterval <= 0 {
		config.HealthCheckInterval = DefaultConfig().HealthCheckInterval
	}
	if config.FailureRateThreshold <= 0 {
		config.FailureRateThreshold = DefaultConfig().FailureRateThreshold
	}
	if config.CriticalFailureRateThreshold <= 0 {
		config.CriticalFailureRateThreshold = DefaultConfig().CriticalFailureRateThreshold
	}
	if config.TimeoutRateThreshold <= 0 {
		config.TimeoutRateThreshold = DefaultConfig().TimeoutRateThreshold
	}

	agg := NewMetricsAggregator(config.MetricsWindowSize)

	obs := &executionObserver{
		config:       config,
		metrics:      agg,
		eventCh:      make(chan Event, config.EventBufferSize),
		recentEvents: make([]Event, config.RecentEventsCapacity),
		stopCh:       make(chan struct{}),
	}

	obs.reporter = NewReporter(agg, config)

	return obs
}

// Emit records an execution event. Non-blocking; if the event buffer is full,
// the event is dropped with a warning log.
func (o *executionObserver) Emit(event Event) {
	// Auto-fill timestamp if not set.
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	select {
	case o.eventCh <- event:
		// Sent successfully.
	default:
		// Buffer full — drop the event to avoid blocking execution.
		logger.Warn("[observer] event buffer full, dropping event: kind=%s, record=%s",
			event.Kind, event.RecordID)
	}
}

// Metrics returns a snapshot of the current aggregated metrics.
func (o *executionObserver) Metrics() ExecutionMetrics {
	return o.metrics.Snapshot()
}

// Report generates an execution report for the given time window.
func (o *executionObserver) Report(window time.Duration) *ExecutionReport {
	recent := o.getRecentEvents()
	return o.reporter.Generate(window, recent)
}

// Start begins the observer's background event processing.
func (o *executionObserver) Start() error {
	o.wg.Add(1)
	go o.processLoop()

	logger.Info("[observer] started (buffer=%d, recent_cap=%d, health_interval=%s)",
		o.config.EventBufferSize, o.config.RecentEventsCapacity, o.config.HealthCheckInterval)
	return nil
}

// Stop gracefully shuts down the observer, draining remaining events.
func (o *executionObserver) Stop() error {
	o.stopOnce.Do(func() {
		close(o.stopCh)
	})
	o.wg.Wait()
	logger.Info("[observer] stopped")
	return nil
}

// processLoop is the single background goroutine that processes events.
func (o *executionObserver) processLoop() {
	defer o.wg.Done()

	for {
		select {
		case <-o.stopCh:
			// Drain remaining events before exiting.
			o.drainEvents()
			return

		case event := <-o.eventCh:
			o.processEvent(event)
		}
	}
}

// processEvent handles a single event: update metrics and store in recent buffer.
func (o *executionObserver) processEvent(event Event) {
	// Update aggregated metrics.
	o.metrics.Record(event)

	// Store in recent events ring buffer.
	o.appendRecent(event)

	// Log significant events.
	switch event.Kind {
	case EventFailed:
		logger.Warn("[observer] SubAgent failed: record=%s, agent=%s, error=%s",
			event.RecordID, event.AgentID, event.Error)
	case EventTimeout:
		logger.Warn("[observer] SubAgent timeout: record=%s, agent=%s, duration=%s",
			event.RecordID, event.AgentID, event.Duration)
	case EventFallback:
		logger.Info("[observer] execution fallback: record=%s, from=%s",
			event.RecordID, event.Strategy)
	case EventCompleted:
		logger.Debug("[observer] SubAgent completed: record=%s, agent=%s, duration=%s",
			event.RecordID, event.AgentID, event.Duration)
	}
}

// appendRecent adds an event to the ring buffer.
func (o *executionObserver) appendRecent(event Event) {
	o.recentMu.Lock()
	defer o.recentMu.Unlock()

	o.recentEvents[o.recentIdx] = event
	o.recentIdx++
	if o.recentIdx >= len(o.recentEvents) {
		o.recentIdx = 0
		o.recentFull = true
	}
}

// getRecentEvents returns a time-ordered copy of recent events.
func (o *executionObserver) getRecentEvents() []Event {
	o.recentMu.RLock()
	defer o.recentMu.RUnlock()

	var events []Event
	if o.recentFull {
		// Ring buffer is full — read from idx to end, then 0 to idx.
		events = make([]Event, len(o.recentEvents))
		n := copy(events, o.recentEvents[o.recentIdx:])
		copy(events[n:], o.recentEvents[:o.recentIdx])
	} else {
		events = make([]Event, o.recentIdx)
		copy(events, o.recentEvents[:o.recentIdx])
	}

	// Filter out zero-value events.
	filtered := events[:0]
	for _, e := range events {
		if e.Kind != "" {
			filtered = append(filtered, e)
		}
	}

	return filtered
}

// drainEvents processes any remaining events in the channel.
func (o *executionObserver) drainEvents() {
	for {
		select {
		case event := <-o.eventCh:
			o.processEvent(event)
		default:
			return
		}
	}
}
