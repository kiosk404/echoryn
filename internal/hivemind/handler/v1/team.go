package v1

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kiosk404/echoryn/internal/hivemind/service/messagebus"
	"github.com/kiosk404/echoryn/internal/hivemind/service/team"
	"github.com/kiosk404/echoryn/internal/pkg/core"
	"github.com/kiosk404/echoryn/pkg/errorx"
)

// TeamHandler handles Team REST API endpoints.
type TeamHandler struct {
	orchestrator    team.TeamOrchestrator
	templateService team.TeamTemplateService
	messageBus      messagebus.MessageBus
}

// NewTeamHandler creates a new TeamHandler.
func NewTeamHandler(
	orch team.TeamOrchestrator,
	tmplSvc team.TeamTemplateService,
	bus messagebus.MessageBus,
) *TeamHandler {
	return &TeamHandler{
		orchestrator:    orch,
		templateService: tmplSvc,
		messageBus:      bus,
	}
}

// ListTemplates handles GET /v1/teams/templates.
func (h *TeamHandler) ListTemplates(c *gin.Context) {
	templates, err := h.templateService.ListTemplates(c.Request.Context(), &team.TemplateFilter{})
	if err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrTeamTemplateList, "list team templates"), nil)
		return
	}

	resp := make([]TeamTemplateResponse, 0, len(templates))
	for _, t := range templates {
		resp = append(resp, toTeamTemplateResponse(t))
	}
	core.WriteResponse(c, nil, gin.H{"data": resp})
}

// CreateTeam handles POST /v1/teams.
func (h *TeamHandler) CreateTeam(c *gin.Context) {
	var req CreateTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrBind, "bind team request"), nil)
		return
	}

	// Get session key from header (parent session).
	parentSessionID := c.GetHeader("X-Session-Key")

	ctx := c.Request.Context()

	var result *team.Team
	var err error

	if req.TemplateID != "" {
		// Template-based creation.
		result, err = h.orchestrator.InstantiateTeam(ctx, &team.InstantiateTeamRequest{
			TemplateID:      req.TemplateID,
			ParentSessionID: parentSessionID,
			TaskDescription: req.TaskDescription,
		})
	} else {
		// Ad-hoc creation.
		strategy := team.CoordinationParallel
		if req.Strategy != "" {
			strategy = team.CoordinationStrategy(req.Strategy)
		}
		result, err = h.orchestrator.CreateTeam(ctx, &team.CreateTeamRequest{
			Name:            req.Name,
			ParentSessionID: parentSessionID,
			TaskDescription: req.TaskDescription,
			Strategy:        strategy,
		})
	}

	if err != nil {
		// Distinguish user-input errors from internal errors.
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "not found"):
			core.WriteResponse(c, errorx.WrapC(err, ErrTeamNotFound, "create team: template not found"), nil)
		case strings.Contains(errMsg, "invalid coordination strategy"):
			core.WriteResponse(c, errorx.WrapC(err, ErrValidation, "create team: invalid strategy"), nil)
		case strings.Contains(errMsg, "failed to spawn any team members"):
			core.WriteResponse(c, errorx.WrapC(err, ErrTeamCreate, "create team: no members could be spawned"), nil)
		default:
			core.WriteResponse(c, errorx.WrapC(err, ErrTeamCreate, "create team"), nil)
		}
		return
	}

	core.WriteResponse(c, nil, toTeamResponse(result))
}

// GetTeam handles GET /v1/teams/:id.
func (h *TeamHandler) GetTeam(c *gin.Context) {
	id := c.Param("id")
	t, err := h.orchestrator.GetTeam(c.Request.Context(), id)
	if err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrTeamNotFound, "team %q not found", id), nil)
		return
	}
	core.WriteResponse(c, nil, toTeamResponse(t))
}

// DissolveTeam handles DELETE /v1/teams/:id.
func (h *TeamHandler) DissolveTeam(c *gin.Context) {
	id := c.Param("id")
	if err := h.orchestrator.DissolveTeam(c.Request.Context(), id); err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrTeamDissolve, "dissolve team %q", id), nil)
		return
	}
	core.WriteResponse(c, nil, gin.H{"id": id, "dissolved": true})
}

// SendMessage handles POST /v1/teams/:id/messages.
func (h *TeamHandler) SendMessage(c *gin.Context) {
	teamID := c.Param("id")

	var req SendTeamMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrBind, "bind message request"), nil)
		return
	}

	ctx := c.Request.Context()

	// Get the team to resolve member labels → session IDs.
	t, err := h.orchestrator.GetTeam(ctx, teamID)
	if err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrTeamNotFound, "team %q not found", teamID), nil)
		return
	}

	if req.Broadcast {
		// Broadcast to all members.
		msg := &messagebus.Message{
			ID:      uuid.New().String(),
			From:    "user",
			TeamID:  teamID,
			Type:    messagebus.MessageTypeChat,
			Content: req.Content,
		}
		if err := h.messageBus.Broadcast(ctx, teamID, msg); err != nil {
			core.WriteResponse(c, errorx.WrapC(err, ErrTeamMessage, "broadcast message"), nil)
			return
		}
		core.WriteResponse(c, nil, gin.H{"sent": true, "broadcast": true, "team_id": teamID})
		return
	}

	// Point-to-point: resolve recipient label → session ID.
	var recipientSessionID string
	for _, m := range t.Members {
		if m.Label == req.Recipient || m.ID == req.Recipient {
			recipientSessionID = m.SessionID
			break
		}
	}
	if recipientSessionID == "" {
		core.WriteResponse(c, errorx.WrapC(nil, ErrTeamMemberNotFound, "member %q not found in team", req.Recipient), nil)
		return
	}

	msg := &messagebus.Message{
		ID:      uuid.New().String(),
		From:    "user",
		To:      recipientSessionID,
		TeamID:  teamID,
		Type:    messagebus.MessageTypeChat,
		Content: req.Content,
	}
	if err := h.messageBus.Send(ctx, msg); err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrTeamMessage, "send message to %q", req.Recipient), nil)
		return
	}

	core.WriteResponse(c, nil, gin.H{"sent": true, "recipient": req.Recipient, "team_id": teamID})
}

// --- Response Builders ---

func toTeamTemplateResponse(t *team.TeamTemplate) TeamTemplateResponse {
	members := make([]TeamTemplateMemberResponse, 0, len(t.MemberSpecs))
	for _, ms := range t.MemberSpecs {
		members = append(members, TeamTemplateMemberResponse{
			ID:       ms.ID,
			Role:     ms.Role,
			Label:    ms.DisplayName,
			IsLeader: ms.IsLeader,
		})
	}
	return TeamTemplateResponse{
		ID:              t.ID,
		Name:            t.Name,
		Description:     t.Description,
		DefaultStrategy: string(t.DefaultStrategy),
		Members:         members,
	}
}

func toTeamResponse(t *team.Team) TeamResponse {
	members := make([]TeamMemberResponse, 0, len(t.Members))
	for _, m := range t.Members {
		members = append(members, TeamMemberResponse{
			ID:        m.ID,
			SessionID: m.SessionID,
			AgentID:   m.AgentID,
			Label:     m.Label,
			Role:      m.Role,
			Status:    string(m.Status),
			IsLeader:  m.ID == t.LeaderID,
			NodeID:    m.NodeID,
			Progress:  m.Progress,
		})
	}
	return TeamResponse{
		ID:       t.ID,
		Name:     t.Name,
		Strategy: string(t.Strategy),
		Status:   string(t.Status),
		Members:  members,
	}
}
