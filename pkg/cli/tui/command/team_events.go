package command

import "context"

// TeamEvent is a client-side DTO representing a real-time team lifecycle event.
// This type is shared between TUI, GUI, and any future consumer.
//
// Design: Decoupled from the server-side team.TeamEvent to allow
// independent evolution of server and client event schemas.
type TeamEvent struct {
	// EventType categorizes the event (e.g., "member_spawned", "member_completed").
	EventType string `json:"event_type"`

	// TeamID identifies the team this event concerns.
	TeamID string `json:"team_id"`

	// MemberID identifies the specific member (if applicable).
	MemberID string `json:"member_id,omitempty"`

	// MemberLabel is a display-friendly label for the member.
	MemberLabel string `json:"member_label,omitempty"`

	// MemberRole is the member's functional role.
	MemberRole string `json:"member_role,omitempty"`

	// MemberStatus is the member's new status after this event.
	MemberStatus string `json:"member_status,omitempty"`

	// Output is the member's final output (for completion events).
	Output string `json:"output,omitempty"`

	// Success indicates whether the outcome was successful (for completion events).
	Success *bool `json:"success,omitempty"`

	// Timestamp is the ISO8601 timestamp of the event.
	Timestamp string `json:"timestamp"`
}

// TeamEventSubscriber is the abstract interface for receiving real-time team events.
//
// Design: This interface decouples event consumers (TUI, GUI, monitoring) from
// the transport mechanism (HTTP SSE, WebSocket, in-process channel).
//
// Implementations:
//   - TeamHTTPSubscriber:  SSE over HTTP (for TUI / CLI clients)
//   - (future) TeamWSSubscriber:   WebSocket (for GUI / web clients)
//   - (future) TeamDirectSubscriber: in-process channel (for embedded use)
type TeamEventSubscriber interface {
	// Subscribe opens a connection and returns a channel of team events.
	// The channel is closed when:
	//   - The context is cancelled
	//   - The server closes the connection
	//   - An unrecoverable error occurs
	//
	// Callers should drain the channel in a goroutine.
	Subscribe(ctx context.Context, teamID string) (<-chan TeamEvent, error)
}
