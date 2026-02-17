package entity

import (
	"fmt"
	"time"
)

// RunStatus represents the lifecycle state of a Run.
//
// State machine: Created → InProgress → Completed | Failed | Cancelled
type RunStatus string

const (
	RunStatusCreated    RunStatus = "created"
	RunStatusInProgress RunStatus = "in_progress"
	RunStatusCompleted  RunStatus = "completed"
	RunStatusFailed     RunStatus = "failed"
	RunStatusCancelled  RunStatus = "cancelled"
)

// IsTerminal returns true if the run has reached a terminal state.
func (s RunStatus) IsTerminal() bool {
	return s == RunStatusCompleted || s == RunStatusFailed || s == RunStatusCancelled
}

// Run represents a single user→agent interaction within a session.
//
// Modeled after:
// - airi-go: RunProcess with status transitions and event emission
// - OpenClaw: a single agent turn (runAgentTurnWithFallback)
type Run struct {
	// ID is the unique run identifier.
	ID string `json:"id"`

	// SessionID is the parent session.
	SessionID string `json:"session_id"`

	// AgentID is the agent executing this run.
	AgentID string `json:"agent_id"`

	// Status is the current lifecycle state.
	Status RunStatus `json:"status"`

	// Input is the user message that triggered this run.
	Input string `json:"input"`

	// Output is the final assistant response (populated on completion).
	Output string `json:"output,omitempty"`

	// Usage tracks token usage for this run.
	Usage *TokenUsage `json:"usage,omitempty"`

	// Error holds error details if the run failed.
	Error *RunError `json:"error,omitempty"`

	// ModelRef records which model actually served this run.
	ModelRef string `json:"model_ref,omitempty"`

	// ToolCallCount is the number of tool calls made during this run.
	ToolCallCount int `json:"tool_call_count,omitempty"`

	// CreatedAt is when this run was created.
	CreatedAt time.Time `json:"created_at"`

	// CompletedAt is when this run reached a terminal state.
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// RunError holds structured error information for a failed run.
type RunError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *RunError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// TokenUsage tracks token consumption for a single run or session.
type TokenUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}
