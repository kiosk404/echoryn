package team

import (
	"time"
)

// TeamEvent represents a domain event that Team BC publishes.
// These events enable other subsystems to observe Team lifecycle without creating dependencies.
//
// Design: Events are the primary inter-domain communication mechanism.
// Other BCs (Collaboration, plugins) subscribe to these events via the event bus.
type TeamEvent struct {
	// EventType categorizes the event.
	EventType TeamEventType `json:"event_type"`

	// TeamID identifies the team this event concerns.
	TeamID string `json:"team_id"`

	// MemberID identifies the team member (if applicable).
	MemberID string `json:"member_id,omitempty"`

	// Timestamp is when the event was generated.
	Timestamp time.Time `json:"timestamp"`

	// Payload contains event-specific data (type depends on EventType).
	Payload interface{} `json:"payload,omitempty"`
}

// TeamEventType categorizes team domain events.
type TeamEventType string

const (
	// TeamEventCreated is emitted when a team is instantiated.
	TeamEventCreated TeamEventType = "team_created"

	// TeamEventMemberSpawned is emitted when a member worker is spawned.
	TeamEventMemberSpawned TeamEventType = "member_spawned"

	// TeamEventMemberStarted is emitted when a member starts executing.
	TeamEventMemberStarted TeamEventType = "member_started"

	// TeamEventMemberProgress is emitted as a member makes progress.
	TeamEventMemberProgress TeamEventType = "member_progress"

	// TeamEventMemberCompleted is emitted when a member completes successfully.
	TeamEventMemberCompleted TeamEventType = "member_completed"

	// TeamEventMemberFailed is emitted when a member fails.
	TeamEventMemberFailed TeamEventType = "member_failed"

	// TeamEventMemberCanceled is emitted when a member is canceled.
	TeamEventMemberCanceled TeamEventType = "member_canceled"

	// TeamEventAllMembersCompleted is emitted when all members reach a terminal state.
	TeamEventAllMembersCompleted TeamEventType = "all_members_completed"

	// TeamEventDissolved is emitted when a team reaches terminal state.
	TeamEventDissolved TeamEventType = "team_dissolved"
)

// MemberSpawnedPayload is the payload for TeamEventMemberSpawned.
type MemberSpawnedPayload struct {
	// MemberID is the team member ID.
	MemberID string `json:"member_id"`

	// WorkerRef is the logical reference to the spawned worker.
	WorkerRef WorkerRef `json:"worker_ref"`

	// AgentID is the agent used by this member.
	AgentID string `json:"agent_id"`

	// Role is the member's role within the team.
	Role string `json:"role"`
}

// MemberCompletedPayload is the payload for member completion events.
type MemberCompletedPayload struct {
	// MemberID is the team member ID.
	MemberID string `json:"member_id"`

	// Status is the terminal status (completed, failed, canceled).
	Status TeamMemberStatus `json:"status"`

	// Output is the member's final output.
	Output string `json:"output,omitempty"`

	// Error is any error message if the member failed.
	Error string `json:"error,omitempty"`
}

// AllMembersCompletedPayload is the payload for TeamEventAllMembersCompleted.
type AllMembersCompletedPayload struct {
	// Results is a map of member ID → output.
	Results map[string]string `json:"results,omitempty"`

	// Success indicates if the team completed successfully.
	Success bool `json:"success"`

	// Error is any team-level error message.
	Error string `json:"error,omitempty"`
}

// TeamPublisher is an optional extension point for publishing Team events.
// Team BC does not require this — it's injected by the integration layer if needed.
//
// Usage:
//   - Plugin layer can inject this to distribute events to subscribers
//   - External systems can hook into team lifecycle this way
//   - Maintains separation: Team BC doesn't know about subscribers
type TeamPublisher interface {
	// PublishTeamEvent emits a team domain event.
	// Implementations must handle errors gracefully (not throw, not block forever).
	PublishTeamEvent(event *TeamEvent)
}

// NoOpTeamPublisher is a default no-op publisher (used when no publisher is registered).
type noOpTeamPublisher struct{}

func (p *noOpTeamPublisher) PublishTeamEvent(event *TeamEvent) {
	// No-op
}

// NewNoOpTeamPublisher creates a no-op publisher.
func NewNoOpTeamPublisher() TeamPublisher {
	return &noOpTeamPublisher{}
}
