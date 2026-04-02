package team

import (
	"context"
)

// TeamRegistry provides persistence for team templates and instances.
//
// Two implementations are provided:
// - InMemoryTeamRegistry: for development/testing
// - BoltDBTeamRegistry: for production persistence
type TeamRegistry interface {
	// --- Template Management ---

	// SaveTemplate persists a team template (create or update).
	SaveTemplate(ctx context.Context, template *TeamTemplate) error

	// GetTemplate retrieves a template by ID
	GetTemplate(ctx context.Context, templateID string) (*TeamTemplate, error)

	// ListTemplates returns templates matching the given filter.
	ListTemplates(ctx context.Context, filter *TemplateFilter) ([]*TeamTemplate, error)

	// DeleteTemplate removes a template by ID.
	DeleteTemplate(ctx context.Context, templateID string) error

	// -- Team Instance Management ---

	// Save persists a team instance (create or update)
	Save(ctx context.Context, team *Team) error

	// Get retrieves a team by ID
	Get(ctx context.Context, teamID string) (*Team, error)

	// ListByParent returns all teams created by a given parent session.
	ListByParent(ctx context.Context, parentSessionID string) ([]*Team, error)

	// Delete removes a team by ID.
	Delete(ctx context.Context, teamID string) error

	// --- Member Management ---

	// AddMember adds a member to a team.
	AddMember(ctx context.Context, teamID string, member *TeamMember) error

	// RemoveMember removes a member from a team.
	RemoveMember(ctx context.Context, teamID string, memberID string) error

	// UpdateMemberStatus updates a member's status.
	UpdateMemberStatus(ctx context.Context, teamID, memberID string, status TeamMemberStatus) error

	// --- Reverse Lookup ---

	// FindBySessionID searches all teams for a member with the given session ID.
	// Returns the team and the member, or nil if not found.
	FindBySessionID(ctx context.Context, sessionID string) (*Team, *TeamMember, error)

	// FindByWorkerRef searches all teams for a member with the given WorkerRef.
	// Returns the team and the member, or nil if not found.
	FindByWorkerRef(ctx context.Context, ref WorkerRef) (*Team, *TeamMember, error)

	// ListAll returns all active (non-dissolved) teams.
	ListAll(ctx context.Context) ([]*Team, error)
}
