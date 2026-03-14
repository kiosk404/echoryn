package bubbletea

import (
	"fmt"
	"time"
)

// =============================================================================
// History Item Interface
// =============================================================================

// HistoryItem represents a single entry in the conversation history.
type HistoryItem interface {
	Timestamp() time.Time
}

// =============================================================================
// Concrete History Item Types
// =============================================================================

// HistoryItemUser represents a user message.
type HistoryItemUser struct {
	Content string
	Time    time.Time
}

// Timestamp returns the message timestamp.
func (h *HistoryItemUser) Timestamp() time.Time {
	return h.Time
}

// HistoryItemAssistant represents an assistant response.
type HistoryItemAssistant struct {
	Content   string
	Time      string // RunID
	Streaming bool
}

// Timestamp returns the message timestamp.
func (h *HistoryItemAssistant) Timestamp() time.Time {
	return time.Now() // RunID is stored in Time field
}

// HistoryItemToolGroup represents a group of tool calls.
type HistoryItemToolGroup struct {
	Calls   []ToolCallInfo
	Results []ToolResultInfo
	Status  ToolGroupStatus
	Time    time.Time
}

// Timestamp returns the message timestamp.
func (h *HistoryItemToolGroup) Timestamp() time.Time {
	return h.Time
}

// ToolCallInfo represents information about a tool call.
type ToolCallInfo struct {
	ID        string
	Name      string
	Arguments string
}

// ToolResultInfo represents the result of a tool execution.
type ToolResultInfo struct {
	CallID string
	Result string
	Error  error
}

// HistoryItemInfo represents a system info/warning/error message.
type HistoryItemInfo struct {
	ID      string
	Content string
	Level   InfoLevel
	Time    time.Time
}

// GetID returns the message ID.
func (h *HistoryItemInfo) GetID() string {
	if h.ID != "" {
		return h.ID
	}
	return fmt.Sprintf("info-%d", h.Time.UnixNano())
}

// Timestamp returns the message timestamp.
func (h *HistoryItemInfo) Timestamp() time.Time {
	return h.Time
}

// HistoryItemTeamMessage represents a message from a team member.
type HistoryItemTeamMessage struct {
	ID      string
	From    string
	Content string
	Time    time.Time
}

// GetID returns the message ID.
func (h *HistoryItemTeamMessage) GetID() string {
	if h.ID != "" {
		return h.ID
	}
	return fmt.Sprintf("team-%s-%d", h.From, h.Time.UnixNano())
}

// Timestamp returns the message timestamp.
func (h *HistoryItemTeamMessage) Timestamp() time.Time {
	return h.Time
}

// =============================================================================
// Enums
// =============================================================================

// ToolGroupStatus represents the status of a group of tool calls.
type ToolGroupStatus int

const (
	ToolGroupPending ToolGroupStatus = iota
	ToolGroupConfirmed
	ToolGroupRunning
	ToolGroupCompleted
	ToolGroupRejected
)

// InfoLevel represents the severity of an info message.
type InfoLevel int

const (
	InfoInfo InfoLevel = iota
	InfoSuccess
	InfoWarning
	InfoError
)

// =============================================================================
// Helper Functions
// =============================================================================

// AddUserMessage adds a user message to the history.
func AddUserMessage(messages []HistoryItem, content string) []HistoryItem {
	return append(messages, &HistoryItemUser{
		Content: content,
		Time:    time.Now(),
	})
}

// AddAssistantMessage adds an assistant message to the history.
func AddAssistantMessage(messages []HistoryItem, content string, streaming bool) []HistoryItem {
	return append(messages, &HistoryItemAssistant{
		Content:   content,
		Streaming: streaming,
	})
}

// AddInfoMessage adds an info message to the history.
func AddInfoMessage(messages []HistoryItem, content string, level InfoLevel) []HistoryItem {
	return append(messages, &HistoryItemInfo{
		Content: content,
		Level:   level,
		Time:    time.Now(),
	})
}

// AddToolGroup adds a tool group to the history.
func AddToolGroup(messages []HistoryItem, calls []ToolCallInfo, status ToolGroupStatus) []HistoryItem {
	return append(messages, &HistoryItemToolGroup{
		Calls:  calls,
		Status: status,
		Time:   time.Now(),
	})
}

// AddTeamMessage adds a team message to the history.
func AddTeamMessage(messages []HistoryItem, from, content string) []HistoryItem {
	return append(messages, &HistoryItemTeamMessage{
		From:    from,
		Content: content,
		Time:    time.Now(),
	})
}
