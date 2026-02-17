package v1

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/entity"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service"
	llmEntity "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	"github.com/kiosk404/echoryn/internal/pkg/core"
	"github.com/kiosk404/echoryn/pkg/errorx"
)

// AgentHandler handles Agent CRUD REST API endpoints.
type AgentHandler struct {
	svc service.AgentService
}

// NewAgentHandler creates a new AgentHandler.
func NewAgentHandler(svc service.AgentService) *AgentHandler {
	return &AgentHandler{svc: svc}
}

// Create handles POST /v1/agents.
func (h *AgentHandler) Create(c *gin.Context) {
	var req CreateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrBind, "bind agent request"), nil)
		return
	}

	agent := &entity.Agent{
		ID:           req.ID,
		Name:         req.Name,
		Description:  req.Description,
		SystemPrompt: req.SystemPrompt,
		Tools:        req.Tools,
		MaxTurns:     req.MaxTurns,
		Temperature:  req.Temperature,
		MaxTokens:    req.MaxTokens,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if req.ModelRef != nil {
		agent.ModelRef = llmEntity.ModelRef{
			ProviderID: req.ModelRef.ProviderID,
			ModelID:    req.ModelRef.ModelID,
		}
	}

	if err := h.svc.CreateAgent(c.Request.Context(), agent); err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrAgentCreate, "create agent"), nil)
		return
	}

	core.WriteResponse(c, nil, toAgentResponse(agent))
}

// List handles GET /v1/agents.
func (h *AgentHandler) List(c *gin.Context) {
	agents, err := h.svc.ListAgents(c.Request.Context())
	if err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrAgentList, "list agents"), nil)
		return
	}

	resp := make([]AgentResponse, 0, len(agents))
	for _, a := range agents {
		resp = append(resp, toAgentResponse(a))
	}
	core.WriteResponse(c, nil, gin.H{"data": resp})
}

// Get handles GET /v1/agents/:id.
func (h *AgentHandler) Get(c *gin.Context) {
	id := c.Param("id")
	agent, err := h.svc.GetAgent(c.Request.Context(), id)
	if err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrAgentNotFound, "agent %q not found", id), nil)
		return
	}
	core.WriteResponse(c, nil, toAgentResponse(agent))
}

// Delete handles DELETE /v1/agents/:id.
func (h *AgentHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.svc.DeleteAgent(c.Request.Context(), id); err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrAgentDelete, "delete agent %q", id), nil)
		return
	}
	core.WriteResponse(c, nil, gin.H{"id": id, "deleted": true})
}

func toAgentResponse(a *entity.Agent) AgentResponse {
	return AgentResponse{
		ID:           a.ID,
		Name:         a.Name,
		Description:  a.Description,
		SystemPrompt: a.SystemPrompt,
		Tools:        a.Tools,
		MaxTurns:     a.MaxTurns,
		CreatedAt:    FormatTime(a.CreatedAt),
		UpdatedAt:    FormatTime(a.UpdatedAt),
	}
}
