package team

import "context"

// TeamApplicationService is the unified Facade for the Team subsystem.
// Plugin layer and external consumers depend on this single interface
// instead of 4+ separate setters (Orchestrator/TemplateService/MessageBus/EventBridge).
//
// This follows the Application Service pattern from DDD:
// - Facade over domain services
// - Transaction boundary
// - Thin layer: delegates to domain objects
type TeamApplicationService interface {
	// --- Team lifecycle ---

	// InstantiateTeam creates a team from a template.
	InstantiateTeam(ctx context.Context, req *InstantiateTeamRequest) (*Team, error)

	// CreateTeam creates an ad-hoc team without a template.
	CreateTeam(ctx context.Context, req *CreateTeamRequest) (*Team, error)

	// DissolveTeam dissolves a team and cleans up resources.
	DissolveTeam(ctx context.Context, teamID string) error

	// GetTeam retrieves a team by ID.
	GetTeam(ctx context.Context, teamID string) (*Team, error)

	// --- Member management ---

	// AddMember adds a new member to an existing team.
	AddMember(ctx context.Context, teamID string, req *AddMemberRequest) (*TeamMember, error)

	// RemoveMember removes a member from a team.
	RemoveMember(ctx context.Context, teamID string, memberID string) error

	// WaitForAll waits for all team members to complete.
	WaitForAll(ctx context.Context, teamID string) (*TeamResult, error)

	// --- Templates ---

	// ListTemplates returns templates matching the given filter.
	ListTemplates(ctx context.Context, filter *TemplateFilter) ([]*TeamTemplate, error)

	// GetTemplate retrieves a template by ID.
	GetTemplate(ctx context.Context, templateID string) (*TeamTemplate, error)
}

// teamAppService is the default implementation of TeamApplicationService.
// It delegates to TeamOrchestrator and TeamTemplateService.
type teamAppService struct {
	orchestrator    TeamOrchestrator
	templateService TeamTemplateService
}

// NewTeamApplicationService creates a Facade that wraps the orchestrator and template service.
func NewTeamApplicationService(
	orchestrator TeamOrchestrator,
	templateService TeamTemplateService,
) TeamApplicationService {
	return &teamAppService{
		orchestrator:    orchestrator,
		templateService: templateService,
	}
}

func (s *teamAppService) InstantiateTeam(ctx context.Context, req *InstantiateTeamRequest) (*Team, error) {
	return s.orchestrator.InstantiateTeam(ctx, req)
}

func (s *teamAppService) CreateTeam(ctx context.Context, req *CreateTeamRequest) (*Team, error) {
	return s.orchestrator.CreateTeam(ctx, req)
}

func (s *teamAppService) DissolveTeam(ctx context.Context, teamID string) error {
	return s.orchestrator.DissolveTeam(ctx, teamID)
}

func (s *teamAppService) GetTeam(ctx context.Context, teamID string) (*Team, error) {
	return s.orchestrator.GetTeam(ctx, teamID)
}

func (s *teamAppService) AddMember(ctx context.Context, teamID string, req *AddMemberRequest) (*TeamMember, error) {
	return s.orchestrator.AddMember(ctx, teamID, req)
}

func (s *teamAppService) RemoveMember(ctx context.Context, teamID string, memberID string) error {
	return s.orchestrator.RemoveMember(ctx, teamID, memberID)
}

func (s *teamAppService) WaitForAll(ctx context.Context, teamID string) (*TeamResult, error) {
	return s.orchestrator.WaitForAll(ctx, teamID)
}

func (s *teamAppService) ListTemplates(ctx context.Context, filter *TemplateFilter) ([]*TeamTemplate, error) {
	return s.templateService.ListTemplates(ctx, filter)
}

func (s *teamAppService) GetTemplate(ctx context.Context, templateID string) (*TeamTemplate, error) {
	return s.templateService.GetTemplate(ctx, templateID)
}

var _ TeamApplicationService = (*teamAppService)(nil)
