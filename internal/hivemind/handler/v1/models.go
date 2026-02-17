package v1

import (
	"github.com/gin-gonic/gin"
	llmService "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/service"
	"github.com/kiosk404/echoryn/internal/pkg/core"
	"github.com/kiosk404/echoryn/pkg/errorx"
)

// ModelHandler handles GET /v1/models (OpenAI-compatible).
//
// Modeled after OpenClaw's openai-http.ts:
type ModelHandler struct {
	manager llmService.ModelManager
}

// NewModelHandler creates a new ModelHandler.
func NewModelHandler(manager llmService.ModelManager) *ModelHandler {
	return &ModelHandler{manager: manager}
}

// List handles GET /v1/models (OpenAI-compatible).
func (h *ModelHandler) List(c *gin.Context) {
	models, err := h.manager.ListAllModels(c.Request.Context())
	if err != nil {
		core.WriteResponse(c, nil, errorx.WrapC(err, ErrModelList, "list models"))
		return
	}
	data := make([]ModelObject, 0, len(models))
	for _, model := range models {
		data = append(data, ModelObject{
			ID:      model.ModelID,
			Object:  "model",
			OwnedBy: model.ProviderID,
		})
	}

	// Also add the virtual "echoryn" model entry.
	data = append(data, ModelObject{
		ID:      "echoryn",
		Object:  "model",
		OwnedBy: "echoryn",
	})

	core.WriteResponse(c, nil, ModelListResponse{
		Object: "list",
		Data:   data,
	})
}
