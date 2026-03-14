// Package client provides client interfaces for the BubbleTea TUI.
// It decouples the TUI from the concrete HTTP/gRPC implementations.
package client

import (
	"context"
)

// StreamClient is the interface for streaming chat communication.
type StreamClient interface {
	// SendMessage sends a message and returns a channel of stream events.
	SendMessage(ctx context.Context, content string) (<-chan StreamEvent, error)

	// Model returns the current model name.
	Model() string

	// SessionID returns the current session ID.
	SessionID() string

	// BaseURL returns the server base URL.
	BaseURL() string
}

// TeamClient is the interface for team management operations.
type TeamClient interface {
	// ListTemplates returns available team templates.
	ListTemplates(ctx context.Context) ([]TemplateInfo, error)

	// CreateTeam creates a new team from a template.
	CreateTeam(ctx context.Context, req CreateTeamRequest) (*TeamInfo, error)

	// GetTeam retrieves team information.
	GetTeam(ctx context.Context, teamID string) (*TeamInfo, error)

	// DissolveTeam dissolves a team.
	DissolveTeam(ctx context.Context, teamID string) error

	// SendMessage sends a message to a team member.
	SendMessage(ctx context.Context, teamID, recipientLabel, content string) error

	// Broadcast sends a message to all team members.
	Broadcast(ctx context.Context, teamID, content string) error

	// Subscribe subscribes to team events.
	Subscribe(ctx context.Context, teamID string) (<-chan TeamEvent, error)
}

// StreamEvent represents an event from the streaming response.
type StreamEvent interface {
	isStreamEvent()
}

// StreamContentEvent represents a content chunk.
type StreamContentEvent struct {
	Delta string
}

func (e StreamContentEvent) isStreamEvent() {}

// StreamToolCallEvent represents tool call requests.
type StreamToolCallEvent struct {
	Calls []ToolCallInfo
}

func (e StreamToolCallEvent) isStreamEvent() {}

// StreamEndEvent represents end of stream.
type StreamEndEvent struct {
	Reason string
}

func (e StreamEndEvent) isStreamEvent() {}

// StreamErrorEvent represents an error.
type StreamErrorEvent struct {
	Error error
}

func (e StreamErrorEvent) isStreamEvent() {}

// ToolCallInfo represents information about a tool call.
type ToolCallInfo struct {
	ID        string
	Name      string
	Arguments string
}

// TemplateInfo represents a team template.
type TemplateInfo struct {
	ID          string
	Name        string
	Description string
	Strategy    string
	MemberCount int
}

// CreateTeamRequest is the request to create a team.
type CreateTeamRequest struct {
	TemplateID      string
	Name            string
	TaskDescription string
	Strategy        string
}

// TeamInfo represents team information.
type TeamInfo struct {
	ID         string
	Name       string
	TemplateID string
	Strategy   string
	Members    []MemberInfo
}

// MemberInfo represents a team member.
type MemberInfo struct {
	ID        string
	SessionID string
	Label     string
	Role      string
	Status    string
	Progress  string
	IsLeader  bool
	NodeID    string
}

// TeamEvent represents an event from the team.
type TeamEvent interface {
	isTeamEvent()
}

// TeamMemberStatusEvent represents a member status change.
type TeamMemberStatusEvent struct {
	SessionID string
	Status    string
	Progress  string
}

func (e TeamMemberStatusEvent) isTeamEvent() {}

// TeamMessageEvent represents a message from a team member.
type TeamMessageEvent struct {
	From      string
	Content   string
	Timestamp int64
}

func (e TeamMessageEvent) isTeamEvent() {}

// TeamDissolvedEvent indicates the team was dissolved.
type TeamDissolvedEvent struct{}

func (e TeamDissolvedEvent) isTeamEvent() {}
