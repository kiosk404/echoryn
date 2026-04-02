package team

import (
	"context"
)

// TeamTemplateService manages team template CRUD operations.
// Templates define reusable team structures that can be instantiated multiple times.
// This is separated from TeamOrchestrator for single responsibility:
// templates are static definitions, while orchestration handles runtime lifecycle.
type TeamTemplateService interface {
	// CreateTemplate creates a new team template.
	CreateTemplate(ctx context.Context, template *TeamTemplate) error

	// GetTemplate retrieves a template by ID.
	GetTemplate(ctx context.Context, templateID string) (*TeamTemplate, error)

	// ListTemplates returns templates matching the given filter.
	ListTemplates(ctx context.Context, filter *TemplateFilter) ([]*TeamTemplate, error)

	// UpdateTemplate updates an existing template.
	UpdateTemplate(ctx context.Context, template *TeamTemplate) error

	// DeleteTemplate removes a template by ID.
	DeleteTemplate(ctx context.Context, templateID string) error
}

// TeamOrchestrator manages team lifecycle and member coordination.
// It is the primary API for creating, managing, and dissolving teams.
//
// Design: K8s controller pattern — watches Team state transitions
// and drives reconciliation (create → active → working → completing → dissolved).
type TeamOrchestrator interface {
	// InstantiateTeam creates a team from a template, spawning all required SubAgents.
	InstantiateTeam(ctx context.Context, req *InstantiateTeamRequest) (*Team, error)

	// CreateTeam creates an ad-hoc team without a template.
	CreateTeam(ctx context.Context, req *CreateTeamRequest) (*Team, error)

	// DissolveTeam dissolves a team, cleaning up all members.
	DissolveTeam(ctx context.Context, teamID string) error

	// GetTeam retrieves a team by ID.
	GetTeam(ctx context.Context, teamID string) (*Team, error)

	// AddMember adds a new member to an existing team.
	AddMember(ctx context.Context, teamID string, req *AddMemberRequest) (*TeamMember, error)

	// RemoveMember removes a member from a team.
	RemoveMember(ctx context.Context, teamID string, memberID string) error

	// ScaleMembers scales the number of instances for a specific member spec.
	ScaleMembers(ctx context.Context, teamID string, specID string, count int) error

	// WaitForAll waits for all team members to complete and returns the aggregated result.
	WaitForAll(ctx context.Context, teamID string) (*TeamResult, error)

	// SetCoordinationStrategy updates the team's coordination strategy.
	SetCoordinationStrategy(ctx context.Context, teamID string, strategy CoordinationStrategy) error

	// NotifyMemberCompleted is called when a member's SubAgent reaches a terminal state.
	// It updates the member's status and auto-signals WaitForAll when all members are done.
	NotifyMemberCompleted(ctx context.Context, teamID string, memberID string, status TeamMemberStatus, output string) error

	// SetExecutionPort wires the outbound port to the Execution domain.
	// This must be called before any team instantiation.
	// Design: dependency injection to maintain loose coupling.
	SetExecutionPort(port ExecutionPort)

	// SetTeamPublisher wires an optional event publisher for Team domain events.
	// If not set, a no-op publisher is used internally.
	SetTeamPublisher(publisher TeamPublisher)
}

// --- Request Types ---

// InstantiateTeamRequest is the input for creating a team from a template.
type InstantiateTeamRequest struct {
	// TemplateID is the template to instantiate.
	TemplateID string `json:"template_id"`

	// ParentSessionID is the session creating this team.
	ParentSessionID string `json:"parent_session_id"`

	// ParentRunID is the run that triggered team creation.
	ParentRunID string `json:"parent_run_id,omitempty"`

	// TaskDescription is the overall task for the team.
	TaskDescription string `json:"task_description"`

	// MemberOverrides allows customizing individual member specs during instantiation.
	MemberOverrides map[string]*MemberOverride `json:"member_overrides,omitempty"`

	// StrategyOverride overrides the template's default strategy.
	StrategyOverride CoordinationStrategy `json:"strategy_override,omitempty"`
}

// MemberOverride allows customizing a member spec during instantiation.
type MemberOverride struct {
	// AgentID overrides the agent to use for this member.
	AgentID string `json:"agent_id,omitempty"`

	// Model overrides the LLM model for this member.
	Model string `json:"model,omitempty"`

	// Task overrides the default task for this member.
	Task string `json:"task,omitempty"`

	// Disabled removes this member from the instantiated team.
	Disabled bool `json:"disabled,omitempty"`
}

// CreateTeamRequest is the input for creating an ad-hoc team (without template).
type CreateTeamRequest struct {
	// Name is the team name.
	Name string `json:"name"`

	// ParentSessionID is the session creating this team.
	ParentSessionID string `json:"parent_session_id"`

	// ParentRunID is the run that triggered team creation.
	ParentRunID string `json:"parent_run_id,omitempty"`

	// TaskDescription is the overall task for the team.
	TaskDescription string `json:"task_description"`

	// Strategy is the coordination strategy.
	Strategy CoordinationStrategy `json:"strategy"`
}

// AddMemberRequest is the input for adding a member to a team.
type AddMemberRequest struct {
	// Role is the functional role for the new member.
	Role string `json:"role"`

	// Label is the display name for the new member.
	Label string `json:"label"`

	// AgentID is the agent to use for this member.
	AgentID string `json:"agent_id,omitempty"`

	// Task is the specific task for this member.
	Task string `json:"task"`

	// Model overrides the LLM model for this member.
	Model string `json:"model,omitempty"`

	// IsLeader indicates if this member should be the team leader.
	IsLeader bool `json:"is_leader,omitempty"`
}
