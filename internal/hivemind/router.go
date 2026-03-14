package hivemind

import (
	"github.com/gin-gonic/gin"
	"github.com/kiosk404/echoryn/internal/hivemind/handler/middleware"
	v1 "github.com/kiosk404/echoryn/internal/hivemind/handler/v1"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service"
	llmService "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/service"
	"github.com/kiosk404/echoryn/internal/hivemind/service/messagebus"
	"github.com/kiosk404/echoryn/internal/hivemind/service/team"
)

// routerDeps holds the dependencies needed for route registration.
type routerDeps struct {
	agentService  service.AgentService
	llmManager    llmService.ModelManager
	authConfig    *middleware.AuthConfig
	gatewayConfig *GatewayConfig

	// Team dependencies (optional - nil when team subsystem is not initialized).
	teamOrchestrator    team.TeamOrchestrator
	teamTemplateService team.TeamTemplateService
	teamMessageBus      messagebus.MessageBus
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

		// Team management (only registered when team subsystem is available).
		if deps.teamOrchestrator != nil && deps.teamTemplateService != nil {
			teamHandler := v1.NewTeamHandler(deps.teamOrchestrator, deps.teamTemplateService, deps.teamMessageBus)
			apiV1.GET("/teams/templates", teamHandler.ListTemplates)
			apiV1.POST("/teams", teamHandler.CreateTeam)
			apiV1.GET("/teams/:id", teamHandler.DissolveTeam)
			apiV1.DELETE("/teams/:id", teamHandler.DissolveTeam)
			apiV1.POST("/teams/:id/messages", teamHandler.SendMessage)
		}
	}
}
