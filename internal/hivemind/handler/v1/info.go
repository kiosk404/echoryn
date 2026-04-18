package v1

import (
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/registry"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/internal/pkg/core"
	"github.com/kiosk404/echoryn/pkg/errorx"
)

// InfoHandler handles the system info endpoints:
//
//	GET /v1/info   — system info (default model, version)
//	GET /v1/tools  — registered tools  (from Plugin Registry)
//	GET /v1/nodes  — online Golem nodes (from Golem Registry)
//	GET /v1/skills — loaded skills      (from skills callback)
type InfoHandler struct {
	pluginRegistry *plugin.Registry
	golemRegistry  registry.Registry                   // nil if Golem not initialized
	skillsFn       func() ([]SkillGroupResponse, int) // nil if skills not available
	defaultModel   string                              // "provider/model" format
}

// NewInfoHandler creates a new InfoHandler.
// All parameters are optional and may be nil.
func NewInfoHandler(
	pluginReg *plugin.Registry,
	golemReg registry.Registry,
	skillsFn func() ([]SkillGroupResponse, int),
	defaultModel string,
) *InfoHandler {
	return &InfoHandler{
		pluginRegistry: pluginReg,
		golemRegistry:  golemReg,
		skillsFn:       skillsFn,
		defaultModel:   defaultModel,
	}
}

// SystemInfo handles GET /v1/info.
func (h *InfoHandler) SystemInfo(c *gin.Context) {
	core.WriteResponse(c, nil, SystemInfoResponse{
		DefaultModel: h.defaultModel,
	})
}

// ListTools handles GET /v1/tools.
func (h *InfoHandler) ListTools(c *gin.Context) {
	if h.pluginRegistry == nil {
		core.WriteResponse(c, nil, ToolsListResponse{})
		return
	}

	tools := h.pluginRegistry.GetTools()

	// Group tools by category.
	grouped := make(map[string][]string)
	for name, td := range tools {
		cat := td.Category
		if cat == "" {
			cat = "other"
		}
		grouped[cat] = append(grouped[cat], name)
	}

	// Sort categories and tool names for stable output.
	var resp ToolsListResponse
	cats := make([]string, 0, len(grouped))
	for cat := range grouped {
		cats = append(cats, cat)
	}
	sort.Strings(cats)

	for _, cat := range cats {
		names := grouped[cat]
		sort.Strings(names)
		resp.Groups = append(resp.Groups, ToolGroupResponse{
			Category: cat,
			Tools:    names,
		})
	}
	resp.Total = len(tools)

	core.WriteResponse(c, nil, resp)
}

// ListNodes handles GET /v1/nodes.
func (h *InfoHandler) ListNodes(c *gin.Context) {
	if h.golemRegistry == nil {
		core.WriteResponse(c, nil, NodesListResponse{})
		return
	}

	// nil filter = return all nodes (any status).
	nodes, err := h.golemRegistry.ListNodes(nil)
	if err != nil {
		core.WriteResponse(c, errorx.WrapC(err, ErrListNodes, "list nodes"), nil)
		return
	}

	var resp NodesListResponse
	for _, n := range nodes {
		resp.Nodes = append(resp.Nodes, NodeInfoResponse{
			ID:     n.Spec.NodeID,
			Name:   n.Spec.NodeName,
			Status: n.Status.Phase.String(),
			Labels: n.Spec.Labels,
		})
	}
	resp.Total = len(resp.Nodes)

	core.WriteResponse(c, nil, resp)
}

// ListSkills handles GET /v1/skills.
func (h *InfoHandler) ListSkills(c *gin.Context) {
	if h.skillsFn == nil {
		core.WriteResponse(c, nil, SkillsListResponse{})
		return
	}

	groups, total := h.skillsFn()
	core.WriteResponse(c, nil, SkillsListResponse{
		Groups: groups,
		Total:  total,
	})
}
