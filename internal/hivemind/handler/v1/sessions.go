package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service"
	"github.com/kiosk404/echoryn/internal/pkg/core"
	"github.com/kiosk404/echoryn/pkg/errorx"
)

// SessionHandler handles Session management REST API endpoints.
type SessionHandler struct {
	svc service.AgentService
}

// NewSessionHandler creates a new SessionHandler.
func NewSessionHandler(svc service.AgentService) *SessionHandler {
	return &SessionHandler{svc: svc}
}

// ListByAgent handles GET /v1/agents/:id/sessions.
func (h *SessionHandler) ListByAgent(c *gin.Context) {
	agentID := c.Param("id")
	sessions, err := h.svc.ListSessionsByAgent(c.Request.Context(), agentID)
	if err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrSessionList, "list sessions for agent %q", agentID), nil)
		return
	}

	resp := make([]SessionResponse, 0, len(sessions))
	for _, s := range sessions {
		resp = append(resp, SessionResponse{
			ID:           s.ID,
			AgentID:      s.AgentID,
			MessageCount: len(s.Messages),
			CreatedAt:    FormatTime(s.CreatedAt),
			UpdatedAt:    FormatTime(s.UpdatedAt),
		})
	}
	core.WriteResponse(c, nil, gin.H{"data": resp})
}

// Get handles GET /v1/sessions/:id.
func (h *SessionHandler) Get(c *gin.Context) {
	id := c.Param("id")
	session, err := h.svc.GetSession(c.Request.Context(), id)
	if err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrSessionNotFound, "session %q not found", id), nil)
		return
	}
	core.WriteResponse(c, nil, SessionResponse{
		ID:           session.ID,
		AgentID:      session.AgentID,
		MessageCount: len(session.Messages),
		CreatedAt:    FormatTime(session.CreatedAt),
		UpdatedAt:    FormatTime(session.UpdatedAt),
	})
}

// Delete handles DELETE /v1/sessions/:id.
func (h *SessionHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.svc.DeleteSession(c.Request.Context(), id); err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrSessionDelete, "delete session %q", id), nil)
		return
	}
	core.WriteResponse(c, nil, gin.H{"id": id, "deleted": true})
}
