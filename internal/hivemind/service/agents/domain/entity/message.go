package entity

import (
	"time"
)

// Role represents the role of a message sender.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message represents a single message in a conversation.
//
// This is our domain model; conversion to/from Eino's schema.Message
// is handled by the message_converter in the runtime layer.
type Message struct {
	// Role is the sender role (system/user/assistant/tool).
	Role Role `json:"role"`

	// Content is the text content of the message.
	Content string `json:"content"`

	// Name is an optional sender name (used for tool results).
	Name string `json:"name,omitempty"`

	// ToolCalls are tool invocations requested by the assistant.
	// Only present when Role == RoleAssistant and the model wants to call tools.
	ToolCalls []*ToolCall `json:"tool_calls,omitempty"`

	// ToolCallID is the ID of the tool call this message responds to.
	// Only present when Role == RoleTool.
	ToolCallID string `json:"tool_call_id,omitempty"`

	// Metadata holds additional information (e.g., model name, latency).
	Metadata map[string]string `json:"metadata,omitempty"`

	// CreatedAt is when this message was created.
	CreatedAt time.Time `json:"created_at"`
}

// NewSystemMessage creates a system message.
func NewSystemMessage(content string) *Message {
	return &Message{
		Role:      RoleSystem,
		Content:   content,
		CreatedAt: time.Now(),
	}
}

// NewUserMessage creates a user message.
func NewUserMessage(content string) *Message {
	return &Message{
		Role:      RoleUser,
		Content:   content,
		CreatedAt: time.Now(),
	}
}

// NewAssistantMessage creates an assistant message.
func NewAssistantMessage(content string) *Message {
	return &Message{
		Role:      RoleAssistant,
		Content:   content,
		CreatedAt: time.Now(),
	}
}

// NewToolMessage creates a tool result message.
func NewToolMessage(toolCallID, name, content string) *Message {
	return &Message{
		Role:       RoleTool,
		Content:    content,
		Name:       name,
		ToolCallID: toolCallID,
		CreatedAt:  time.Now(),
	}
}
