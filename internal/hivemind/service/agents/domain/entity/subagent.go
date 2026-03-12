package entity

import (
	"fmt"
	"time"
)

// SubAgentStatus represents the lifecycle state of a spawned sub-agent.
//
// State machine (K8S Pod Phase equivalent):
//
//	Pending → Running → Completed | Failed | Cancelled
type SubAgentStatus string

const (
	SubAgentStatusPending   SubAgentStatus = "pending"
	SubAgentStatusRunning   SubAgentStatus = "running"
	SubAgentStatusCompleted SubAgentStatus = "completed"
	SubAgentStatusFailed    SubAgentStatus = "failed"
	SubAgentStatusCancelled SubAgentStatus = "cancelled"
)

// IsTerminal returns true if the sub-agent has reached a terminal state.
func (s SubAgentStatus) IsTerminal() bool {
	return s == SubAgentStatusCompleted || s == SubAgentStatusFailed || s == SubAgentStatusCancelled
}

// SubAgentOutcome classifies the terminal result of a sub-agent run.
// Modeled after OpenClaw's SubagentRunOutcome.
type SubAgentOutcome string

const (
	SubAgentOutcomeOK      SubAgentOutcome = "ok"
	SubAgentOutcomeError   SubAgentOutcome = "error"
	SubAgentOutcomeTimeout SubAgentOutcome = "timeout"
	SubAgentOutcomeUnknown SubAgentOutcome = "unknown"
)

// SubAgentSpawnRequest is the request to spawn a sub-agent from a parent session.
//
// Modeled after OpenClaw's sessions_spawn:
//   - Parent agent delegates a task to a sub-agent running in an independent session
//   - Sub-agent can use a different (cheaper) model
//   - Sub-agent cannot spawn further sub-agents (max depth = 1)
//   - On completion, result is announced back to the parent session
type SubAgentSpawnRequest struct {
	// ParentSessionID is the session that initiated the spawn.
	ParentSessionID string `json:"parent_session_id"`

	// ParentRunID is the run within the parent session that triggered the spawn.
	ParentRunID string `json:"parent_run_id"`

	// AgentID is the agent to run as the sub-agent.
	// If empty, uses the same agent as the parent.
	AgentID string `json:"agent_id,omitempty"`

	// Task is the instruction / prompt for the sub-agent.
	Task string `json:"task"`

	// Label is an optional human-readable label for this sub-agent.
	Label string `json:"label,omitempty"`

	// Model overrides the model used by the sub-agent (e.g., "openai:gpt-4o-mini").
	// If empty, inherits from the parent agent or defaults config.
	Model string `json:"model,omitempty"`

	// Cleanup controls what happens to the sub-agent session after completion.
	// "delete" removes the session immediately; "keep" preserves it for inspection.
	// Default: "delete".
	Cleanup string `json:"cleanup,omitempty"`
}

// EffectiveCleanup returns the cleanup strategy, defaulting to "delete".
func (r *SubAgentSpawnRequest) EffectiveCleanup() string {
	if r.Cleanup == "keep" {
		return "keep"
	}
	return "delete"
}

// SubAgentRecord tracks a spawned sub-agent's lifecycle.
//
// Stored in SubAgentRegistry for persistence across process restarts.
// Analogous to OpenClaw's SubagentRunRecord + K8S Pod status tracking.
type SubAgentRecord struct {
	// ID is the unique identifier for this sub-agent instance.
	ID string `json:"id"`

	// ParentSessionID is the session that spawned this sub-agent.
	ParentSessionID string `json:"parent_session_id"`

	// ParentRunID is the run that triggered the spawn.
	ParentRunID string `json:"parent_run_id"`

	// SessionID is the independent session created for this sub-agent's execution.
	SessionID string `json:"session_id"`

	// AgentID is the agent running as the sub-agent.
	AgentID string `json:"agent_id"`

	// Task is the instruction given to the sub-agent.
	Task string `json:"task"`

	// Label is an optional human-readable label.
	Label string `json:"label,omitempty"`

	// Model is the model used by the sub-agent (may differ from parent).
	Model string `json:"model,omitempty"`

	// Cleanup is the cleanup strategy ("delete" or "keep").
	Cleanup string `json:"cleanup,omitempty"`

	// Index is the 0-based spawn order within the parent session.
	// Assigned at spawn time based on the count of existing children.
	// Used for fuzzy matching: "subagent-2" -> Index=2
	// Aligned with OpenClaw's index-based subagent lookup.
	Index int `json:"index"`

	// SpawnDepth tracks the nesting depth of this sub-agent (1-based).
	// A depth of 1 means it was spawned directly by a top-level session.
	// Aligned with OpenClaw's spawnDepth in SubagentRunRecord.
	SpawnDepth int `json:"spawn_depth,omitempty"`

	// Status is the current lifecycle state.
	Status SubAgentStatus `json:"status"`

	// Outcome classifies the terminal result.
	Outcome SubAgentOutcome `json:"outcome,omitempty"`

	// Result holds the sub-agent's output on completion.
	Result string `json:"result,omitempty"`

	// Error holds error details if the sub-agent failed.
	Error string `json:"error,omitempty"`

	// Announced is true once the result has been delivered to the parent session.
	Announced bool `json:"announced"`

	// CreatedAt is when this sub-agent was spawned.
	CreatedAt time.Time `json:"created_at"`

	// StartedAt is when the sub-agent began running.
	StartedAt *time.Time `json:"started_at,omitempty"`

	// CompletedAt is when this sub-agent reached a terminal state.
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// Duration returns the wall-clock duration of this sub-agent run.
// Returns 0 if not yet started or completed.
func (r *SubAgentRecord) Duration() time.Duration {
	if r.StartedAt == nil {
		return 0
	}
	end := time.Now()
	if r.CompletedAt != nil {
		end = *r.CompletedAt
	}
	return end.Sub(*r.StartedAt)
}

// MarkRunning transitions the record to running state.
func (r *SubAgentRecord) MarkRunning() {
	now := time.Now()
	r.Status = SubAgentStatusRunning
	r.StartedAt = &now
}

// MarkCompleted transitions the record to a terminal state.
func (r *SubAgentRecord) MarkCompleted(result string) {
	now := time.Now()
	r.Status = SubAgentStatusCompleted
	r.Outcome = SubAgentOutcomeOK
	r.Result = result
	r.CompletedAt = &now
}

// MarkFailed transitions the record to failed state.
func (r *SubAgentRecord) MarkFailed(errMsg string) {
	now := time.Now()
	r.Status = SubAgentStatusFailed
	r.Outcome = SubAgentOutcomeError
	r.Error = errMsg
	r.CompletedAt = &now
}

// MarkCancelled transitions the record to cancelled state.
func (r *SubAgentRecord) MarkCancelled() {
	now := time.Now()
	r.Status = SubAgentStatusCancelled
	r.Outcome = SubAgentOutcomeUnknown
	r.CompletedAt = &now
}

// FormatAnnouncement builds the announcement message injected into the parent session.
// Modeled after OpenClaw's buildSubagentAnnounceMessage.
func (r *SubAgentRecord) FormatAnnouncement() string {
	status := "completed"
	if r.Status == SubAgentStatusFailed {
		status = "failed"
	} else if r.Status == SubAgentStatusCancelled {
		status = "cancelled"
	}

	dur := r.Duration()
	header := fmt.Sprintf("[SubAgent Report] Task: %s\nStatus: %s | Duration: %s",
		r.Task, status, dur.Round(time.Second))

	body := r.Result
	if r.Status == SubAgentStatusFailed {
		body = fmt.Sprintf("Error: %s", r.Error)
	}

	return fmt.Sprintf("%s\n\n%s", header, body)
}
