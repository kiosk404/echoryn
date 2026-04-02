// Package team provides PolicyService for centralized team policy governance.
package team

import "time"

// PolicyService centralizes team operational policies.
// Borrowing from deer-flow's unified governance pattern:
// global defaults + per-team overrides in a single entry point.
//
// Previously these values were scattered across orchestratorImpl fields
// and hardcoded constants. PolicyService collects them here.
type PolicyService interface {
	// MaxTeamMembers returns the maximum number of members allowed in a team.
	MaxTeamMembers(teamID string) int

	// MaxSpawnDepth returns the maximum SubAgent recursion depth.
	MaxSpawnDepth() int

	// MemberTimeout returns the timeout for a single member's execution.
	MemberTimeout(teamID, memberID string) time.Duration

	// CanSpawnNestedTeam returns whether a team member is allowed to create sub-teams.
	// Default: false (prevent recursive team nesting, similar to deer-flow's subagent_enabled=False).
	CanSpawnNestedTeam() bool
}

// DefaultPolicyService provides sensible defaults.
// Override per-team values by wrapping or replacing this implementation.
type DefaultPolicyService struct {
	maxMembers    int
	maxDepth      int
	memberTimeout time.Duration
	allowNested   bool
}

// NewDefaultPolicyService creates a PolicyService with production defaults.
func NewDefaultPolicyService() *DefaultPolicyService {
	return &DefaultPolicyService{
		maxMembers:    DefaultMaxTeamMembers, // 8
		maxDepth:      3,
		memberTimeout: 10 * time.Minute,
		allowNested:   false,
	}
}

// PolicyOption is a functional option for DefaultPolicyService.
type PolicyOption func(*DefaultPolicyService)

// WithMaxMembers overrides the max team members.
func WithMaxMembers(n int) PolicyOption {
	return func(p *DefaultPolicyService) { p.maxMembers = n }
}

// WithMaxDepth overrides the max spawn depth.
func WithMaxDepth(n int) PolicyOption {
	return func(p *DefaultPolicyService) { p.maxDepth = n }
}

// WithMemberTimeout overrides the default member timeout.
func WithMemberTimeout(d time.Duration) PolicyOption {
	return func(p *DefaultPolicyService) { p.memberTimeout = d }
}

// WithNestedTeams enables or disables nested team creation.
func WithNestedTeams(allow bool) PolicyOption {
	return func(p *DefaultPolicyService) { p.allowNested = allow }
}

// NewPolicyService creates a PolicyService with the given options.
func NewPolicyService(opts ...PolicyOption) *DefaultPolicyService {
	p := NewDefaultPolicyService()
	for _, o := range opts {
		o(p)
	}
	return p
}

func (p *DefaultPolicyService) MaxTeamMembers(_ string) int             { return p.maxMembers }
func (p *DefaultPolicyService) MaxSpawnDepth() int                      { return p.maxDepth }
func (p *DefaultPolicyService) MemberTimeout(_, _ string) time.Duration { return p.memberTimeout }
func (p *DefaultPolicyService) CanSpawnNestedTeam() bool                { return p.allowNested }

var _ PolicyService = (*DefaultPolicyService)(nil)
