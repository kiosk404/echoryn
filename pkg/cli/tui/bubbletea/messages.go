package bubbletea

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// =============================================================================
// User Interaction Messages
// =============================================================================

// InputMsg represents user input submission.
type InputMsg struct {
	Content string
}

// Timestamp returns the message timestamp.
func (m InputMsg) Timestamp() time.Time {
	return time.Now()
}

// =============================================================================
// Streaming Messages
// =============================================================================

// StreamStartMsg indicates the start of a streaming response.
type StreamStartMsg struct {
	RunID string
}

// StreamChunkMsg represents a chunk of streaming content.
type StreamChunkMsg struct {
	Content string
	Done    bool
}

// StreamEndMsg indicates the end of a streaming response.
type StreamEndMsg struct {
	RunID  string
	Reason string // "complete", "error", "cancelled"
}

// StreamErrorMsg represents an error during streaming.
type StreamErrorMsg struct {
	Error error
}

// =============================================================================
// Tool Execution Messages
// =============================================================================

// ToolCallRequestMsg indicates tool calls are pending user confirmation.
type ToolCallRequestMsg struct {
	Calls []ToolCallInfo
}

// ToolConfirmationMsg represents user's tool confirmation decision.
type ToolConfirmationMsg struct {
	Approved bool
	CallIDs  []string
}

// ToolResultMsg represents the result of a tool execution.
type ToolResultMsg struct {
	CallID string
	Result string
	Error  error
}

// =============================================================================
// Team Messages
// =============================================================================

// TeamCreatedMsg indicates a team was successfully created.
type TeamCreatedMsg struct {
	TeamID   string
	Name     string
	Template string
	Members  []MemberState
}

// TeamDissolvedMsg indicates the current team was dissolved.
type TeamDissolvedMsg struct {
	TeamID string
}

// MemberStatusMsg indicates a member's status changed.
type MemberStatusMsg struct {
	SessionID string
	Status    MemberStatus
	Progress  string
}

// TeamMessageMsg represents a message from a team member.
type TeamMessageMsg struct {
	From      string
	Content   string
	Timestamp time.Time
}

// =============================================================================
// UI State Messages
// =============================================================================

// TickMsg is sent periodically for animations.
type TickMsg time.Time

// QuitMsg indicates the user wants to quit.
type QuitMsg struct{}

// ErrorMsg represents a generic error to display.
type ErrorMsg struct {
	Error error
	Time  time.Time
}

// InfoMsg represents a message to display to the user.
type InfoMsg struct {
	Content string
	Time    time.Time
}

// =============================================================================
// Commands
// =============================================================================

// quitCmd returns a command that quits the program.
func quitCmd() tea.Cmd {
	return tea.Quit
}

// tickCmd returns a command that sends a tick message.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}
