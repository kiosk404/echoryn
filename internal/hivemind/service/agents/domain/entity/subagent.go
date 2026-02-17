package entity

import (
	"time"
)

// SubAgentStatus represents the lifecycle state of a spawned sub-agent.
//
// State machine: Pending → Running → Completed | Failed | Cancelled
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

// SubAgentSpawnRequest is the request to spawn a sub-agent from a parent session.
//
// Modeled after OpenClaw's sessions_spawn:
//   - Parent agent delegates a task to a sub-agent running in an independent session
//   - Sub-agent can use a different (cheaper) model
//   - Sub-agent cannot spawn further sub-agents (max depth = 1)
//   - On completion, result is announced back to the parent session
//
// TODO(subagent): Implement spawning logic in AgentService + SubAgentManager.
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

	// SystemPrompt overrides the sub-agent's system prompt if provided.
	SystemPrompt string `json:"system_prompt,omitempty"`
}

// SubAgentRecord tracks a spawned sub-agent's lifecycle.
//
// Stored in SubAgentRegistry for persistence across process restarts.
// Analogous to OpenClaw's SubagentRegistry entries.
//
// TODO(subagent): Implement SubAgentRegistry (BoltDB-backed) for persistence.
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

	// Status is the current lifecycle state.
	Status SubAgentStatus `json:"status"`

	// Result holds the sub-agent's output on completion.
	Result string `json:"result,omitempty"`

	// Error holds error details if the sub-agent failed.
	Error string `json:"error,omitempty"`

	// CreatedAt is when this sub-agent was spawned.
	CreatedAt time.Time `json:"created_at"`

	// CompletedAt is when this sub-agent reached a terminal state.
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}
