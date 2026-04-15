package bubbletea

import "time"

// ─────────────────────────────────────────────────────────────────────────────
// tea.Msg types for the ChatModel state machine.
//
// Each message represents an event that drives a state transition.
// Named with "Msg" suffix per BubbleTea convention.
// ─────────────────────────────────────────────────────────────────────────────

// --- Streaming lifecycle ---

// StreamStartMsg signals that streaming has been initiated.
// Sent by the streaming tea.Cmd immediately before the HTTP call.
type StreamStartMsg struct{}

// StreamDeltaMsg delivers an incremental text chunk from the SSE stream.
type StreamDeltaMsg struct {
	Delta string
}

// StreamToolCallMsg signals that the LLM is invoking a tool.
type StreamToolCallMsg struct {
	Name string
}

// StreamDoneMsg signals that streaming has completed (successfully or with error).
type StreamDoneMsg struct {
	Content string
	Usage   *ChatTokenUsage
	Err     error
}

// --- Rendering ---

// RenderDoneMsg signals that markdown re-rendering is complete.
type RenderDoneMsg struct {
	Rendered string
	Raw      string
}

// --- Spinner ---

// SpinnerTickMsg drives the thinking spinner animation.
type SpinnerTickMsg struct {
	Time time.Time
}

// --- Command execution ---

// CommandResultMsg carries the result of a slash command execution.
type CommandResultMsg struct {
	Output string
	Err    error
	IsQuit bool
}

// --- Token usage ---

// ChatTokenUsage tracks token consumption for a single chat interaction.
// Local definition to avoid import cycle with the render package.
type ChatTokenUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}
