package scheduler

import (
	"time"
)

// ScheduleMode defines how a Golem node is selected for task execution.
// Two modes are supported:
//   - DirectMode: The caller explicitly specifies a target Golem by node ID.
//   - AIMode: The scheduler's AI-driven selector autonomously picks the best Golem
//     based on capabilities, system resources, installed skills, and current load.
type ScheduleMode string

const (
	// DirectMode indicates that the caller has explicitly chosen a specific Golem node.
	DirectMode ScheduleMode = "direct"

	// AIMode indicates that an AI-driven selector will autonomously pick the best Golem.
	AIMode ScheduleMode = "ai"
)

// ScheduleRequest encapsulates everything the scheduler needs to dispatch a task.
// It carries both the task itself and the scheduling preferences (mode, constraints, hints).
type ScheduleRequest struct {
	// Task is the work unit to be scheduled.
	Task *protocol.Task

	// Mode controls how the Golem node is selected.
	Mode ScheduleMode

	// TargetNodeID is the explicit node ID when Mode is DirectMode.
	// Ignored when Mode is AIMode.
	TargetNodeID string

	// RequiredCapabilities lists the capabilities that the target Golem must advertise.
	// In DirectMode, this is used for validation; in AIMode, for filtering candidates.
	RequiredCapabilities []string

	// RequiredSkills lists the skills that must be installed on the target Golem.
	RequiredSkills []string

	// RequiredFeatures lists the features that the target Golem must support.
	RequiredFeatures []string

	// PreferredTags are soft preferences for node selection (e.g., {"region": "us-west"}).
	// Matching tags increase a node's score but are not mandatory.
	PreferredTags map[string]string

	// ResourceRequirements specifies minimum system resource thresholds.
	ResourceRequirements *ResourceRequirements

	// Hints provides additional context for the AI selector to make better decisions.
	Hints *ScheduleHints

	// RequestedAt records when the scheduling request was created.
	RequestedAt time.Time
}

// ResourceRequirements specifies the minimum system resources a Golem node must have
// to be eligible for a task. Values of 0 mean "no constraint".
type ResourceRequirements struct {
	// MinCPUCores is the minimum number of CPU cores required.
	MinCPUCores int

	// MinMemoryMB is the minimum free memory in megabytes.
	MinMemoryMB int64

	// MinDiskFreeMB is the minimum free disk space in megabytes.
	MinDiskFreeMB int64

	// MaxCPUPercent is the maximum acceptable CPU utilisation percentage (0-100).
	MaxCPUPercent float64

	// MaxMemoryPercent is the maximum acceptable memory utilisation percentage (0-100).
	MaxMemoryPercent float64

	// MaxActiveTasks is the maximum number of concurrent tasks the node may already be running.
	MaxActiveTasks int
}

// ScheduleHints provides supplementary context that the AI selector can use
// to refine its ranking of candidate Golem nodes.
type ScheduleHints struct {
	// Description is a human-readable summary of what the task does,
	// allowing the AI to reason about which node is best suited.
	Description string

	// PreferLowLatency indicates that the caller favours a node with the lowest
	// expected response time (e.g., fewest queued tasks).
	PreferLowLatency bool

	// PreferHighResources indicates that the caller favours a node with the most
	// available resources (CPU, memory, disk).
	PreferHighResources bool

	// Affinity is a node ID hint suggesting the scheduler should prefer this node
	// if it meets all hard constraints. Useful for session stickiness.
	Affinity string

	// AntiAffinity is a set of node IDs that should be avoided if possible.
	AntiAffinity []string

	// CustomContext is arbitrary key-value metadata the AI selector may inspect.
	CustomContext map[string]string
}

// ScheduleDecision records the outcome of the scheduling process.
type ScheduleDecision struct {
	// RequestID is the unique identifier linking this decision to a ScheduleRequest.
	RequestID string

	// Mode is the scheduling mode that was used.
	Mode ScheduleMode

	// SelectedNodeID is the ID of the Golem node that was chosen.
	SelectedNodeID string

	// Reason is a human-readable explanation of why this node was selected.
	// Particularly useful in AIMode where the reasoning may be non-trivial.
	Reason string

	// Scores contains the per-node scoring breakdown (only populated in AIMode).
	Scores []NodeScore

	// CandidateCount is the total number of nodes that were evaluated.
	CandidateCount int

	// EligibleCount is the number of nodes that passed all hard constraints.
	EligibleCount int

	// DecidedAt records when the decision was finalised.
	DecidedAt time.Time

	// Latency is the wall-clock time it took to reach the decision.
	Latency time.Duration
}

// NodeScore captures the scoring breakdown for a single candidate node.
type NodeScore struct {
	// NodeID identifies the Golem node.
	NodeID string

	// TotalScore is the weighted aggregate score (higher is better).
	TotalScore float64

	// CapabilityScore reflects how well the node's capabilities match the request.
	CapabilityScore float64

	// SkillScore reflects how many of the required skills are installed.
	SkillScore float64

	// ResourceScore reflects the node's available system resources.
	ResourceScore float64

	// LoadScore reflects how busy the node currently is (lower load = higher score).
	LoadScore float64

	// TagScore reflects how many preferred tags match.
	TagScore float64

	// AffinityScore reflects whether the node matches affinity/anti-affinity hints.
	AffinityScore float64

	// Eligible indicates whether this node passed all hard constraints.
	Eligible bool

	// RejectReason explains why the node was rejected, if Eligible is false.
	RejectReason string
}

// GolemProfile aggregates the static and dynamic information about a Golem node
// that the scheduler uses for decision-making. It is a denormalised snapshot
// assembled from the cluster registry, heartbeat data, and capability reports.
type GolemProfile struct {
	// NodeInfo is the static registration data for the Golem.
	NodeInfo protocol.NodeInfo

	// Load is the most recent load report from the Golem's heartbeat.
	Load protocol.NodeLoadInfo

	// InstalledSkills lists the skills currently installed on this Golem.
	InstalledSkills []SkillInfo

	// SupportedFeatures lists the high-level features this Golem supports
	// (e.g., "browser_automation", "gpu_inference", "sandbox_execution").
	SupportedFeatures []string

	// Tags are user-defined labels attached to the Golem for filtering.
	Tags map[string]string

	// HealthScore is a composite health indicator (0.0 = dead, 1.0 = perfect).
	HealthScore float64

	// LastUpdated records when this profile was last refreshed.
	LastUpdated time.Time
}

// SkillInfo describes a skill installed on a Golem node.
type SkillInfo struct {
	// ID is the unique identifier of the skill.
	ID string

	// Name is the human-readable skill name.
	Name string

	// Version is the semantic version of the installed skill.
	Version string

	// Capabilities lists the capabilities that this skill provides.
	Capabilities []string
}

// TaskEvent represents a lifecycle event in the task scheduling pipeline.
type TaskEvent struct {
	// Type identifies the kind of event.
	Type TaskEventType

	// Task is the task associated with this event.
	Task *protocol.Task

	// Decision is the scheduling decision (only set for EventTypeAssigned).
	Decision *ScheduleDecision

	// NodeID identifies the Golem node involved (empty for queue-only events).
	NodeID string

	// Progress is the incremental progress report (only set for EventTypeProgress).
	Progress *protocol.TaskProgress

	// Result is the final result (only set for EventTypeCompleted).
	Result *protocol.TaskResult

	// Error captures the failure reason (only set for EventTypeFailed).
	Error error

	// Timestamp records when the event occurred.
	Timestamp time.Time
}

// TaskEventType enumerates the kinds of task lifecycle events.
type TaskEventType string

const (
	// EventTypeSubmitted is emitted when a task enters the scheduling queue.
	EventTypeSubmitted TaskEventType = "submitted"

	// EventTypeAssigned is emitted when a task is assigned to a Golem node.
	EventTypeAssigned TaskEventType = "assigned"

	// EventTypeProgress is emitted when incremental progress is reported.
	EventTypeProgress TaskEventType = "progress"

	// EventTypeCompleted is emitted when a task finishes successfully.
	EventTypeCompleted TaskEventType = "completed"

	// EventTypeFailed is emitted when a task fails.
	EventTypeFailed TaskEventType = "failed"

	// EventTypeCancelled is emitted when a task is cancelled.
	EventTypeCancelled TaskEventType = "cancelled"

	// EventTypeTimedOut is emitted when a task exceeds its timeout.
	EventTypeTimedOut TaskEventType = "timed_out"

	// EventTypeRescheduled is emitted when a task is re-queued after a node failure.
	EventTypeRescheduled TaskEventType = "rescheduled"
)

// TaskEventListener receives notifications about task lifecycle transitions.
// Implementations must be goroutine-safe as events may fire from multiple goroutines.
type TaskEventListener interface {
	// OnEvent is called for every task lifecycle event.
	OnEvent(event *TaskEvent)
}

// TaskEventListenerFunc is an adapter that allows ordinary functions to serve
// as TaskEventListener. Follows the http.HandlerFunc pattern.
type TaskEventListenerFunc func(event *TaskEvent)

// OnEvent calls the wrapped function.
func (f TaskEventListenerFunc) OnEvent(event *TaskEvent) {
	f(event)
}

// ScheduleRequestBuilder provides a fluent API for constructing ScheduleRequest instances.
// Follows the Builder pattern for ergonomic request creation.
type ScheduleRequestBuilder struct {
	request *ScheduleRequest
}

// NewScheduleRequest creates a new ScheduleRequestBuilder with sensible defaults.
func NewScheduleRequest(task *protocol.Task) *ScheduleRequestBuilder {
	return &ScheduleRequestBuilder{
		request: &ScheduleRequest{
			Task:        task,
			Mode:        AIMode,
			RequestedAt: time.Now(),
		},
	}
}

// WithDirectMode switches to direct scheduling, targeting a specific Golem node.
func (b *ScheduleRequestBuilder) WithDirectMode(nodeID string) *ScheduleRequestBuilder {
	b.request.Mode = DirectMode
	b.request.TargetNodeID = nodeID
	return b
}

// WithAIMode switches to AI-driven scheduling (this is the default).
func (b *ScheduleRequestBuilder) WithAIMode() *ScheduleRequestBuilder {
	b.request.Mode = AIMode
	b.request.TargetNodeID = ""
	return b
}

// WithRequiredCapabilities sets the capabilities the target Golem must advertise.
func (b *ScheduleRequestBuilder) WithRequiredCapabilities(caps ...string) *ScheduleRequestBuilder {
	b.request.RequiredCapabilities = caps
	return b
}

// WithRequiredSkills sets the skills that must be installed on the target Golem.
func (b *ScheduleRequestBuilder) WithRequiredSkills(skills ...string) *ScheduleRequestBuilder {
	b.request.RequiredSkills = skills
	return b
}

// WithRequiredFeatures sets the features that the target Golem must support.
func (b *ScheduleRequestBuilder) WithRequiredFeatures(features ...string) *ScheduleRequestBuilder {
	b.request.RequiredFeatures = features
	return b
}

// WithPreferredTags sets soft preferences for node tags.
func (b *ScheduleRequestBuilder) WithPreferredTags(tags map[string]string) *ScheduleRequestBuilder {
	b.request.PreferredTags = tags
	return b
}

// WithResourceRequirements sets minimum resource thresholds.
func (b *ScheduleRequestBuilder) WithResourceRequirements(req *ResourceRequirements) *ScheduleRequestBuilder {
	b.request.ResourceRequirements = req
	return b
}

// WithHints sets the scheduling hints for the AI selector.
func (b *ScheduleRequestBuilder) WithHints(hints *ScheduleHints) *ScheduleRequestBuilder {
	b.request.Hints = hints
	return b
}

// Build returns the constructed ScheduleRequest.
func (b *ScheduleRequestBuilder) Build() *ScheduleRequest {
	return b.request
}
