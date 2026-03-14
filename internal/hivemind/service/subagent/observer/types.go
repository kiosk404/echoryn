// Package observer provides execution observability for SubAgent tasks.
// It collects execution metrics, emits lifecycle events, and generates
// summary reports for monitoring and debugging purposes.
//
// Architecture:
//
//	Observer (facade)
//	  ├── Collector   (event ingestion + buffered channel)
//	  ├── Metrics     (aggregated counters + histograms)
//	  └── Reporter    (snapshot generation + query API)
//
// Design principles:
//   - Non-blocking: all operations are fire-and-forget; observer failures
//     never block or degrade SubAgent execution.
//   - Thread-safe: all state is protected by mutexes or atomic operations.
//   - Low overhead: uses buffered channels and periodic aggregation.
//
// Integration points:
//   - executor/local.go:  emits events during local execution
//   - executor/golem.go:  emits events during remote scheduling/execution
//   - domain/subagent/executor.go: emits lifecycle events (spawn/running/completed/failed)
package observer

import (
	"time"
)

// --- Event Types ---

// EventKind identifies the category of an observer event.
type EventKind string

const (
	// EventSpawned is emitted when a SubAgent is spawned (record created).
	EventSpawned EventKind = "spawned"

	// EventScheduled is emitted when a SubAgent is submitted to the scheduler.
	EventScheduled EventKind = "scheduled"

	// EventRunning is emitted when a SubAgent transitions to running state.
	EventRunning EventKind = "running"

	// EventCompleted is emitted when a SubAgent finishes successfully.
	EventCompleted EventKind = "completed"

	// EventFailed is emitted when a SubAgent fails.
	EventFailed EventKind = "failed"

	// EventCancelled is emitted when a SubAgent is cancelled.
	EventCancelled EventKind = "cancelled"

	// EventTimeout is emitted when a SubAgent exceeds its execution timeout.
	EventTimeout EventKind = "timeout"

	// EventRouted is emitted when the ExecutionRouter selects an executor.
	EventRouted EventKind = "routed"

	// EventFallback is emitted when execution falls back (e.g., golem → local).
	EventFallback EventKind = "fallback"

	// EventStreamError is emitted when a stream consumption error occurs.
	EventStreamError EventKind = "stream_error"

	// EventAnnounced is emitted when a SubAgent result is announced to the parent.
	EventAnnounced EventKind = "announced"
)

// ExecutionLocation identifies where a SubAgent is executing.
type ExecutionLocation string

const (
	LocationLocal ExecutionLocation = "local"
	LocationGolem ExecutionLocation = "golem"
)

// --- Core Event ---

// Event is a single observable occurrence during SubAgent execution.
// Events are emitted by various components and consumed by the Collector.
type Event struct {
	// Kind identifies the event category.
	Kind EventKind `json:"kind"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// RecordID is the SubAgent record ID (correlates with SubAgentRecord.ID).
	RecordID string `json:"record_id"`

	// SessionID is the SubAgent's session ID.
	SessionID string `json:"session_id,omitempty"`

	// ParentSessionID is the parent session that spawned this SubAgent.
	ParentSessionID string `json:"parent_session_id,omitempty"`

	// AgentID is the agent running as the SubAgent.
	AgentID string `json:"agent_id,omitempty"`

	// TeamID is the team context (if any).
	TeamID string `json:"team_id,omitempty"`

	// Location is where the SubAgent is executing.
	Location ExecutionLocation `json:"location,omitempty"`

	// Strategy is the execution strategy used.
	Strategy string `json:"strategy,omitempty"`

	// NodeID is the Golem node ID (for remote execution).
	NodeID string `json:"node_id,omitempty"`

	// Duration is the elapsed time for this phase (e.g., scheduling latency, execution time).
	Duration time.Duration `json:"duration,omitempty"`

	// Error contains error details (for failure events).
	Error string `json:"error,omitempty"`

	// SpawnDepth is the nesting depth of this SubAgent.
	SpawnDepth int `json:"spawn_depth,omitempty"`

	// Metadata holds arbitrary key-value pairs for extensibility.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// --- Aggregated Metrics ---

// ExecutionMetrics holds aggregated metrics for SubAgent execution.
// This is a value type (DTO) — safe to copy and return from Snapshot().
type ExecutionMetrics struct {
	// Counters
	TotalSpawned   int64 `json:"total_spawned"`
	TotalCompleted int64 `json:"total_completed"`
	TotalFailed    int64 `json:"total_failed"`
	TotalCancelled int64 `json:"total_cancelled"`
	TotalTimeout   int64 `json:"total_timeout"`
	TotalFallback  int64 `json:"total_fallback"`

	// Execution location breakdown
	LocalExecutions int64 `json:"local_executions"`
	GolemExecutions int64 `json:"golem_executions"`

	// Current state
	CurrentRunning int64 `json:"current_running"`
	CurrentPending int64 `json:"current_pending"`

	// Timing (rolling window averages)
	AvgExecutionTime   time.Duration `json:"avg_execution_time"`
	AvgScheduleLatency time.Duration `json:"avg_schedule_latency"`
	MaxExecutionTime   time.Duration `json:"max_execution_time"`
	P95ExecutionTime   time.Duration `json:"p95_execution_time"`

	// Per-agent breakdown
	AgentMetrics map[string]*AgentExecutionMetrics `json:"agent_metrics,omitempty"`

	// Per-team breakdown (only for team-scoped SubAgents)
	TeamMetrics map[string]*TeamExecutionMetrics `json:"team_metrics,omitempty"`

	// Window timestamps
	WindowStart time.Time `json:"window_start"`
	CollectedAt time.Time `json:"collected_at"`
}

// AgentExecutionMetrics holds per-agent execution statistics.
type AgentExecutionMetrics struct {
	AgentID        string        `json:"agent_id"`
	TotalSpawned   int64         `json:"total_spawned"`
	TotalCompleted int64         `json:"total_completed"`
	TotalFailed    int64         `json:"total_failed"`
	AvgExecTime    time.Duration `json:"avg_exec_time"`
	SuccessRate    float64       `json:"success_rate"`
}

// TeamExecutionMetrics holds per-team execution statistics.
type TeamExecutionMetrics struct {
	TeamID          string        `json:"team_id"`
	TotalTasks      int64         `json:"total_tasks"`
	CompletedTasks  int64         `json:"completed_tasks"`
	FailedTasks     int64         `json:"failed_tasks"`
	AvgExecTime     time.Duration `json:"avg_exec_time"`
	MemberCount     int           `json:"member_count"`
	ActiveNodeCount int           `json:"active_node_count"`
}

// --- Observer Interface ---

// Observer is the top-level facade for SubAgent execution observability.
// It is injected into executor components and the domain layer to collect
// execution events without creating tight coupling.
//
// Design: All methods are non-blocking and safe to call from hot paths.
// If the observer is nil, callers should skip emit calls (nil-safe pattern).
type Observer interface {
	// Emit records an execution event. Non-blocking; events are buffered
	// and processed asynchronously.
	Emit(event Event)

	// Metrics returns the current aggregated metrics snapshot.
	// The returned value is a copy — safe to read without synchronization.
	Metrics() ExecutionMetrics

	// Report generates a summary report for the given time window.
	Report(window time.Duration) *ExecutionReport

	// Start begins the observer's background processing.
	Start() error

	// Stop gracefully shuts down the observer.
	Stop() error
}

// --- Report ---

// ExecutionReport is a point-in-time summary of SubAgent execution health.
// Generated by the Reporter and used by:
//   - TeamOrchestrator: to assess team member health
//   - API layer: to expose observability endpoints
//   - Diagnostics: to debug execution issues
type ExecutionReport struct {
	// Summary
	Window        time.Duration    `json:"window"`
	Metrics       ExecutionMetrics `json:"metrics"`
	HealthStatus  HealthStatus     `json:"health_status"`
	HealthMessage string           `json:"health_message"`

	// Recent events (last N events for debugging)
	RecentEvents []Event `json:"recent_events,omitempty"`

	// Alerts (active anomalies)
	Alerts []Alert `json:"alerts,omitempty"`

	// Generated at
	GeneratedAt time.Time `json:"generated_at"`
}

// HealthStatus classifies the overall SubAgent execution health.
type HealthStatus string

const (
	HealthStatusHealthy  HealthStatus = "healthy"
	HealthStatusDegraded HealthStatus = "degraded"
	HealthStatusCritical HealthStatus = "critical"
)

// Alert represents an active anomaly detected by the observer.
type Alert struct {
	// Severity: "warning" or "critical"
	Severity string `json:"severity"`

	// Type identifies the alert category.
	Type string `json:"type"`

	// Message is a human-readable description.
	Message string `json:"message"`

	// DetectedAt is when the alert was first raised.
	DetectedAt time.Time `json:"detected_at"`

	// Context holds additional data for debugging.
	Context map[string]string `json:"context,omitempty"`
}

// --- Configuration ---

// Config holds configuration for the observer module.
type Config struct {
	// EventBufferSize is the capacity of the event channel.
	// Default: 256.
	EventBufferSize int

	// RecentEventsCapacity is how many recent events to retain for reports.
	// Default: 100.
	RecentEventsCapacity int

	// MetricsWindowSize is the rolling window size for timing calculations.
	// Default: 500 samples.
	MetricsWindowSize int

	// HealthCheckInterval is how often the reporter evaluates health status.
	// Default: 30s.
	HealthCheckInterval time.Duration

	// FailureRateThreshold triggers a "degraded" alert when exceeded.
	// Default: 0.3 (30%).
	FailureRateThreshold float64

	// CriticalFailureRateThreshold triggers a "critical" alert.
	// Default: 0.6 (60%).
	CriticalFailureRateThreshold float64

	// TimeoutRateThreshold triggers a timeout alert.
	// Default: 0.2 (20%).
	TimeoutRateThreshold float64
}

// DefaultConfig returns sensible defaults for the observer.
func DefaultConfig() Config {
	return Config{
		EventBufferSize:              256,
		RecentEventsCapacity:         100,
		MetricsWindowSize:            500,
		HealthCheckInterval:          30 * time.Second,
		FailureRateThreshold:         0.3,
		CriticalFailureRateThreshold: 0.6,
		TimeoutRateThreshold:         0.2,
	}
}
