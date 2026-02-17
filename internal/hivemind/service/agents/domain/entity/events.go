package entity

// EventType identifies the type of a streaming agent event.
//
// Modeled after:
// - airi-go: AgentEvent with EventType enum (chat_model_answer, func_call, tools_message, etc.)
// - OpenClaw: SSE event types in the streaming pipeline
type EventType string

const (
	// EventTextDelta is a chunk of assistant text being streamed.
	EventTextDelta EventType = "text_delta"

	// EventToolCallStart indicates a tool call has been initiated.
	EventToolCallStart EventType = "tool_call_start"

	// EventToolCallEnd indicates a tool call has completed.
	EventToolCallEnd EventType = "tool_call_end"

	// EventRunStatus indicates a run status change.
	EventRunStatus EventType = "run_status"

	// EventError indicates an error occurred during the run.
	EventError EventType = "error"

	// EventDone indicates the run has completed and the stream is ending.
	EventDone EventType = "done"

	// EventSubAgentSpawned indicates a sub-agent has been spawned.
	// TODO(subagent): Emit this event when SubAgentManager.Spawn() succeeds.
	EventSubAgentSpawned EventType = "subagent_spawned"

	// EventSubAgentCompleted indicates a sub-agent has finished and announced its result.
	// TODO(subagent): Emit this event when sub-agent result is injected into parent session.
	EventSubAgentCompleted EventType = "subagent_completed"
)

// AgentEvent is a streaming event emitted during agent execution.
//
// This flows through schema.Pipe[*AgentEvent] from the execution goroutine
// to the client-facing stream, following the airi-go pattern.
type AgentEvent struct {
	// Type identifies which kind of event this is.
	Type EventType `json:"type"`

	// Delta contains the text chunk for EventTextDelta events.
	Delta string `json:"delta,omitempty"`

	// ToolCall contains tool call info for EventToolCallStart events.
	ToolCall *ToolCall `json:"tool_call,omitempty"`

	// ToolResult contains tool execution result for EventToolCallEnd events.
	ToolResult *ToolResult `json:"tool_result,omitempty"`

	// RunStatus contains the new status for EventRunStatus events.
	RunStatus RunStatus `json:"run_status,omitempty"`

	// Error contains the error message for EventError events.
	Error string `json:"error,omitempty"`

	// Usage contains token usage information for EventDone events.
	Usage *TokenUsage `json:"usage,omitempty"`

	// SubAgentID is the sub-agent record ID for EventSubAgentSpawned/EventSubAgentCompleted.
	// TODO(subagent): Populate when emitting sub-agent events.
	SubAgentID string `json:"subagent_id,omitempty"`

	// SubAgentResult is the sub-agent's output for EventSubAgentCompleted events.
	// TODO(subagent): Populate when sub-agent announces result back to parent.
	SubAgentResult string `json:"subagent_result,omitempty"`
}
