package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service"
)

// RunsHandler handles run lifecycle operations via REST API.
type RunsHandler struct {
	svc service.AgentService
}

// NewRunsHandler creates a new RunsHandler.
func NewRunsHandler(svc service.AgentService) *RunsHandler {
	return &RunsHandler{svc: svc}
}

// Abort handles DELETE /v1/runs/:id
// Cancels a running agent execution by ID.
//
// Response:
//   - 200 OK: {"ok": true} — abort signal sent successfully
//   - 200 OK: {"ok": true, "note": "..."} — run not found (may have already completed)
func (h *RunsHandler) Abort(c *gin.Context) {
	runID := c.Param("id")
	if runID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run ID is required"})
		return
	}

	err := h.svc.AbortRun(c.Request.Context(), runID)
	if err != nil {
		// "not found" is not really an error — the run may have finished already.
		// Return 200 so the client knows to stop waiting.
		c.JSON(http.StatusOK, gin.H{"ok": true, "note": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
