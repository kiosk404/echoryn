package hivemind

import (
	"context"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/kiosk404/echoryn/internal/hivemind/handler/middleware"
	v1 "github.com/kiosk404/echoryn/internal/hivemind/handler/v1"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/registry"
	llmService "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/service"
	"github.com/kiosk404/echoryn/internal/hivemind/service/messagebus"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/internal/hivemind/service/team"
	pkgskills "github.com/kiosk404/echoryn/pkg/skills"
)

// routerDeps holds the dependencies needed for route registration.
type routerDeps struct {
	agentService  service.AgentService
	llmManager    llmService.ModelManager
	authConfig    *middleware.AuthConfig
	gatewayConfig *GatewayConfig

	// Info endpoint dependencies (optional - nil when not initialized).
	pluginFramework *plugin.Framework
	golemRegistry   registry.Registry

	// Team dependencies (optional - nil when team subsystem is not initialized).
	teamOrchestrator    team.TeamOrchestrator
	teamTemplateService team.TeamTemplateService
	teamMessageBus      messagebus.MessageBus
	teamPublisher       *team.ChannelTeamPublisher
}

func initRouter(g *gin.Engine, deps *routerDeps) {
	installMiddleware(g, deps)
	installController(g, deps)
}

func installMiddleware(g *gin.Engine, deps *routerDeps) {
	g.Use(gin.Recovery())
	g.Use(middleware.CORS())

	if deps.authConfig != nil {
		g.Use(middleware.BearerAuth(deps.authConfig))
	}
}

func installController(g *gin.Engine, deps *routerDeps) {
	defaultAgentID := "main"
	defaultModel := "echoryn"
	if deps.gatewayConfig != nil {
		if deps.gatewayConfig.Defaults.AgentID != "" {
			defaultAgentID = deps.gatewayConfig.Defaults.AgentID
		}
		if deps.gatewayConfig.Defaults.Model != "" {
			defaultModel = deps.gatewayConfig.Defaults.Model
		}
	}

	// Handlers.
	chatHandler := v1.NewChatCompletionsHandler(deps.agentService, deps.llmManager, defaultAgentID, defaultModel)
	agentHandler := v1.NewAgentHandler(deps.agentService)
	sessionHandler := v1.NewSessionHandler(deps.agentService)
	modelHandler := v1.NewModelHandler(deps.llmManager)

	// InfoHandler: tools + nodes + skills + system info.
	var pluginReg *plugin.Registry
	if deps.pluginFramework != nil {
		pluginReg = deps.pluginFramework.Registry()
	}
	// Resolve the default model name for the /v1/info endpoint.
	resolvedDefaultModel := resolveDefaultModelName(deps)
	infoHandler := v1.NewInfoHandler(pluginReg, deps.golemRegistry, buildSkillsFn(deps.pluginFramework), resolvedDefaultModel)

	// --- /v1 route group ---
	apiV1 := g.Group("/v1")
	{
		// OpenAI-compatible endpoints.
		apiV1.POST("/chat/completions", chatHandler.Handle)
		apiV1.GET("/models", modelHandler.List)

		// Agent CRUD.
		apiV1.POST("/agents", agentHandler.Create)
		apiV1.GET("/agents", agentHandler.List)
		apiV1.GET("/agents/:id", agentHandler.Get)
		apiV1.DELETE("/agents/:id", agentHandler.Delete)

		// Session management.
		apiV1.GET("/agents/:id/sessions", sessionHandler.ListByAgent)
		apiV1.GET("/sessions/:id", sessionHandler.Get)
		apiV1.DELETE("/sessions/:id", sessionHandler.Delete)

		// System info endpoints.
		apiV1.GET("/info", infoHandler.SystemInfo)
		apiV1.GET("/tools", infoHandler.ListTools)
		apiV1.GET("/nodes", infoHandler.ListNodes)
		apiV1.GET("/skills", infoHandler.ListSkills)

		// Run lifecycle (abort)

		// Team management (only registered when team subsystem is available).
		if deps.teamOrchestrator != nil && deps.teamTemplateService != nil {
			teamHandler := v1.NewTeamHandler(deps.teamOrchestrator, deps.teamTemplateService, deps.teamMessageBus, deps.teamPublisher)
			apiV1.GET("/teams/templates", teamHandler.ListTemplates)
			apiV1.POST("/teams", teamHandler.CreateTeam)
			apiV1.GET("/teams/:id", teamHandler.GetTeam)
			apiV1.DELETE("/teams/:id", teamHandler.DissolveTeam)
			apiV1.POST("/teams/:id/messages", teamHandler.SendMessage)
			apiV1.GET("/teams/:id/events", teamHandler.SubscribeEvents)
		}
	}
}

// buildSkillsFn creates a closure that retrieves skills grouped by source
// from the skills plugin within the plugin framework.
// Returns nil if the framework or skills plugin is not available.
func buildSkillsFn(fw *plugin.Framework) func() ([]v1.SkillGroupResponse, int) {
	if fw == nil {
		return nil
	}

	// Try to get the skills plugin and extract its registry.
	// The skills plugin implements an internal interface — we query it via
	// the plugin registry's GetPlugin and then use a type assertion on the
	// exported SkillsRegistryProvider interface we define here.
	type skillsRegistryProvider interface {
		SkillsRegistry() *pkgskills.Registry
	}

	p, ok := fw.Registry().GetPlugin("skills")
	if !ok {
		return nil
	}
	provider, ok := p.(skillsRegistryProvider)
	if !ok {
		return nil
	}
	reg := provider.SkillsRegistry()
	if reg == nil {
		return nil
	}

	return func() ([]v1.SkillGroupResponse, int) {
		metadata := reg.GetMetadata()

		grouped := make(map[string][]string)
		for _, m := range metadata {
			src := string(m.Source)
			if src == "" {
				src = "unknown"
			}
			grouped[src] = append(grouped[src], m.Name)
		}

		sources := make([]string, 0, len(grouped))
		for src := range grouped {
			sources = append(sources, src)
		}
		sort.Strings(sources)

		groups := make([]v1.SkillGroupResponse, 0, len(sources))
		for _, src := range sources {
			names := grouped[src]
			sort.Strings(names)
			groups = append(groups, v1.SkillGroupResponse{
				Source: src,
				Skills: names,
			})
		}

		return groups, len(metadata)
	}
}

// resolveDefaultModelName queries the LLM manager for the configured default
// model and returns it as "provider/model" (e.g., "hunyuan/hunyuan-turbos-latest").
func resolveDefaultModelName(deps *routerDeps) string {
	if deps.llmManager == nil {
		return ""
	}
	m, err := deps.llmManager.GetDefaultModel(context.Background())
	if err != nil || m == nil {
		return ""
	}
	if m.ProviderID != "" {
		return m.ProviderID + "/" + m.ModelID
	}
	return m.ModelID
}
