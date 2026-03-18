package hivemind

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/config"
	grpchandler "github.com/kiosk404/echoryn/internal/hivemind/handler/grpc"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents"
	agentService "github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/prompt"
	"github.com/kiosk404/echoryn/internal/hivemind/service/gateway"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/registry"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/scheduler"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/tokenmanager"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm"
	llmEntity "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/entity"
	llmService "github.com/kiosk404/echoryn/internal/hivemind/service/llm/domain/service"
	"github.com/kiosk404/echoryn/internal/hivemind/service/mcp"
	"github.com/kiosk404/echoryn/internal/hivemind/service/messagebus"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin"
	"github.com/kiosk404/echoryn/internal/hivemind/service/team"
	genericapiserver "github.com/kiosk404/echoryn/internal/pkg/server"
	"github.com/kiosk404/echoryn/pkg/http/shutdown"
	"github.com/kiosk404/echoryn/pkg/http/shutdown/posixsignal"
	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/paths"
	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
	"google.golang.org/grpc"
)

// ============================================================
// Section 1: InitFunc + Chain
// ============================================================

// InitFunc is a function that initializes a subsystem.
type InitFunc func(ctx context.Context, deps *Dependencies) error

// InitializerChain executes a sequence of init functions in order.
type InitializerChain []InitFunc

// Run executes all init functions in sequence.
func (c InitializerChain) Run(ctx context.Context, deps *Dependencies) error {
	for i, init := range c {
		if err := init(ctx, deps); err != nil {
			return fmt.Errorf("initializer[%d]: %w", i, err)
		}
	}
	return nil
}

// ============================================================
// Section 2: Dependencies - Shared Resources
// ============================================================

// Dependencies holds all initialized modules and resources.
type Dependencies struct {
	Config *config.Config

	// Core infrastructure
	Shutdown      *shutdown.GracefulShutdown
	GenericServer *genericapiserver.GenericAPIServer
	GRPCServer    *genericapiserver.GRPCAPIServer
	GenericConfig *genericapiserver.Config
	ExtraConfig   *ExtraConfig

	// Core modules
	Golem  *golem.Module
	LLM    *llm.Module
	MCP    *mcp.Module
	Agents *agents.Module
	Plugin *plugin.Framework

	// Team subsystem
	TeamOrchestrator    team.TeamOrchestrator
	TeamTemplateService team.TeamTemplateService
	TeamMessageBus      messagebus.MessageBus

	// Gateway
	ChannelManager *gateway.ChannelManager
}

// ============================================================
// Section 3: Infrastructure Initializer
// ============================================================

// InitInfrastructure creates state dir, shutdown manager, and servers.
func InitInfrastructure(cfg *config.Config) InitFunc {
	return func(ctx context.Context, deps *Dependencies) error {
		// 1. State directory
		stateDir, err := paths.EnsureStateDir()
		if err != nil {
			return fmt.Errorf("failed to ensure state directory: %w", err)
		}
		logger.Info("[Hivemind] state directory: %s", stateDir)

		// 2. Shutdown manager
		gs := shutdown.New()
		gs.AddShutdownManager(posixsignal.NewPosixSignalManager())
		deps.Shutdown = gs

		// 3. Generic server
		genericConfig, err := buildGenericConfig(cfg)
		if err != nil {
			return err
		}
		deps.GenericConfig = genericConfig
		genericServer, err := genericConfig.Complete().New()
		if err != nil {
			return err
		}
		deps.GenericServer = genericServer

		// 4. Extra config
		extraConfig, err := buildExtraConfig(cfg)
		if err != nil {
			return err
		}
		deps.ExtraConfig = extraConfig

		return nil
	}
}

// ============================================================
// Section 4: Core Modules Initializers
// ============================================================

// InitGolem creates the Golem subsystem.
func InitGolem() InitFunc {
	return func(ctx context.Context, deps *Dependencies) error {
		golemCfg := &golem.Config{
			Registry:  registry.Config{},
			Scheduler: scheduler.DefaultSchedulerConfig(),
			TokenManager: tokenmanager.Config{
				DefaultTTL:    24 * time.Hour,
				MaxTTL:        168 * time.Hour,
				CleanupPeriod: time.Hour,
			},
		}

		golemModule, err := golemCfg.Complete().New()
		if err != nil {
			return fmt.Errorf("failed to initialize Golem subsystem: %w", err)
		}
		if err := golemModule.Start(ctx); err != nil {
			return fmt.Errorf("failed to start Golem subsystem: %w", err)
		}
		deps.Golem = golemModule
		logger.Info("[Hivemind] Golem subsystem initialized successfully (admin_token available)")

		// Persist admin token
		adminTokenPath := paths.ResolveAdminTokenPath()
		if err := os.WriteFile(adminTokenPath, []byte(golemModule.TokenManager.AdminToken()), 0o600); err != nil {
			logger.Warn("[Hivemind] failed to write admin token to %s:%v", adminTokenPath, err)
		} else {
			logger.Info("[Hivemind] Admin token saved to: %s", adminTokenPath)
		}

		// Create gRPC server with auth interceptors
		extraServer, err := deps.ExtraConfig.complete().New(
			grpc.ChainUnaryInterceptor(grpchandler.AdminAuthUnaryInterceptor(golemModule.TokenManager)),
			grpc.ChainStreamInterceptor(grpchandler.AdminAuthStreamInterceptor(golemModule.TokenManager)),
		)
		if err != nil {
			return err
		}
		deps.GRPCServer = extraServer

		// Register Golem gRPC services
		devMode := deps.Config.GolemOptions != nil && deps.Config.GolemOptions.DevMode
		nodeServiceHandler := grpchandler.NewNodeServiceHandler(golemModule.Registry, golemModule.TokenManager, devMode)
		adminServiceHandler := grpchandler.NewAdminServiceHandler(golemModule.Registry, golemModule.TokenManager)
		pb.RegisterGolemNodeServiceServer(extraServer.Server, nodeServiceHandler)
		pb.RegisterHivemindAdminServiceServer(extraServer.Server, adminServiceHandler)
		golemModule.BindStreamManager(nodeServiceHandler)
		logger.Info("[Hivemind] Golem gRPC services registered (GolemNodeService + HivemindAdminService + stream-based dispatch)")
		if devMode {
			logger.Warn("[Hivemind] Golem dev-mode ENABLED: loopback nodes can register without join token")
		}

		return nil
	}
}

// InitLLM creates the LLM module.
func InitLLM() InitFunc {
	return func(ctx context.Context, deps *Dependencies) error {
		llmCfg := &llm.Config{
			ModelOptions: deps.Config.ModelOptions,
		}
		llmModule, err := llmCfg.Complete().New(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize LLM module: %w", err)
		}
		deps.LLM = llmModule
		logger.Info("LLM module initialized successfully")

		// Bind LLM manager to Golem scheduler
		if deps.Golem != nil {
			if err := deps.Golem.BindLLMManager(llmModule.Manager); err != nil {
				logger.Warn("[Hivemind] failed to bind LLM manager to Golem scheduler: %v", err)
			} else {
				logger.Info("[Hivemind] LLM-enhanced scheduling enabled for Golem subsystem")
			}
		}

		return nil
	}
}

// InitMCP creates the MCP module.
func InitMCP() InitFunc {
	return func(ctx context.Context, deps *Dependencies) error {
		mcpFileCfg, err := mcp.LoadMCPConfig(deps.Config.MCPOptions.ConfigFile)
		if err != nil {
			return fmt.Errorf("failed to load MCP config from %q: %w", deps.Config.MCPOptions.ConfigFile, err)
		}
		mcpCfg := &mcp.Config{
			MCPConfig: mcpFileCfg,
		}
		mcpModule, err := mcpCfg.Complete().New(ctx)
		if err != nil {
			return fmt.Errorf("failed to create MCP module: %w", err)
		}
		deps.MCP = mcpModule
		logger.Info("[Hivemind] MCP module initialized successfully")
		return nil
	}
}

// InitAgents creates the plugin framework and agents module.
func InitAgents() InitFunc {
	return func(ctx context.Context, deps *Dependencies) error {
		// 1. Create plugin framework
		pluginCfg := &plugin.Config{
			SlotConfig: plugin.SlotConfig{
				"memory":  deps.Config.PluginOptions.Slots.Memory,
				"channel": deps.Config.PluginOptions.Slots.Channel,
			},
			RuntimeAPI: plugin.NewRuntimeAPI(&modelManagerAdapter{deps.LLM.Manager}),
		}
		pluginFramework := pluginCfg.Complete().New()
		pluginFramework.SetPromptPipeline(prompt.NewDefaultPipeline())
		deps.Plugin = pluginFramework

		// 2. Create agents module
		agentsCfg := &agents.Config{}
		agentsModule, err := agentsCfg.Complete().New(ctx, agents.Dependencies{
			LLM:     deps.LLM,
			Plugins: deps.Plugin,
			MCP:     deps.MCP.Manager,
		})
		if err != nil {
			return fmt.Errorf("failed to create Agents module: %w", err)
		}
		deps.Agents = agentsModule
		logger.Info("[Hivemind] Agents module initialized successfully")

		return nil
	}
}

// ============================================================
// Section 5: Plugin Lifecycle & Interface Probes
// ============================================================

// InitPluginLifecycle runs plugin init/start and performs all injections.
func InitPluginLifecycle() InitFunc {
	return func(ctx context.Context, deps *Dependencies) error {
		// Early channel gateway setup
		if err := initChannelGateway(deps); err != nil {
			return err
		}

		if !deps.Config.PluginOptions.Enabled {
			logger.Info("[Hivemind] Plugin framework disabled (plugins.enabled=false), skipping plugin loading")
			return nil
		}

		// Register in-tree plugins
		inTreeRegistry := builtin.NewInTreeRegistry(deps.Config.PluginOptions, deps.Golem)
		if err := inTreeRegistry.ApplyTo(deps.Plugin); err != nil {
			return fmt.Errorf("failed to register in-tree plugins: %w", err)
		}

		// Initialize plugins
		if err := deps.Plugin.Init(); err != nil {
			return fmt.Errorf("failed to initialize plugin framework: %w", err)
		}

		// Inject SubAgentManager
		injectInterface(deps.Plugin, func(p plugin.Plugin) bool {
			if setter, ok := p.(interface {
				SetManager(mgr agentService.SubAgentManager)
			}); ok {
				setter.SetManager(deps.Agents.SubAgentManager)
				return true
			}
			return false
		}, "SubAgentManager")

		// Inject Team dependencies
		if err := injectTeamDependencies(deps); err != nil {
			return err
		}

		// Re-init channel gateway (ensures injection before Start)
		if err := initChannelGateway(deps); err != nil {
			return err
		}

		// Start plugins
		if err := deps.Plugin.Start(ctx); err != nil {
			return fmt.Errorf("failed to start plugin framework: %w", err)
		}
		logger.Info("[Hivemind] Plugin framework initialized successfully (%d plugins loaded)",
			deps.Plugin.Registry().Len())
		return nil
	}
}

// injectInterface is a generic helper for interface probe injection.
func injectInterface(pf *plugin.Framework, injector func(p plugin.Plugin) bool, name string) {
	reg := pf.Registry()
	for _, n := range reg.PluginNames() {
		if p, ok := reg.GetPlugin(n); ok && injector(p) {
			logger.Info("[Hivemind] injected %s into plugin %q", name, n)
			return
		}
	}
	logger.Debug("[Hivemind] no %s found among initialized plugins", name)
}

// injectTeamDependencies creates the Team subsystem and injects into plugins.
func injectTeamDependencies(deps *Dependencies) error {
	// 1. Create registry and template service
	teamRegistry := team.NewInMemoryTeamRegistry()
	templateSvc := team.NewTemplateService(teamRegistry)

	// 2. Load templates
	templateDirs := paths.ResolveTemplatesDirs()
	loader := team.NewTemplateLoader(teamRegistry, templateDirs...)
	count, err := loader.LoadAll(context.Background())
	if err != nil {
		logger.Warn("[Hivemind] template loading errors: %v", err)
	}
	if count > 0 {
		logger.Info("[Hivemind] loaded %d team templates from %v", count, templateDirs)
	}

	// 3. Create MessageBus and Orchestrator
	bus := messagebus.NewMessageBus(nil)
	gatewayCfg := DefaultGatewayConfig()
	orchestrator := team.NewOrchestrator(teamRegistry, templateSvc, deps.Agents.SubAgentManager, deps.Agents.SessionRepo, bus, gatewayCfg.Defaults.AgentID)

	// 4. Wire resolver
	if settable, ok := bus.(messagebus.ResolverSetter); ok {
		settable.SetTeamMemberResolver(orchestrator.(messagebus.TeamMemberResolver))
	}

	// 5. Set up TranscriptPlugin
	setupTeamTranscript(deps.Plugin, bus)

	// 6. Create EventBridge
	eventBridge := team.NewEventBridge(orchestrator, teamRegistry)
	deps.Agents.SubAgentManager.SetLifecycleHook(eventBridge)
	if setter, ok := orchestrator.(interface {
		SetEventBridge(bridge *team.EventBridge)
	}); ok {
		setter.SetEventBridge(eventBridge)
	}

	// 7. Inject into plugins
	reg := deps.Plugin.Registry()
	for _, name := range reg.PluginNames() {
		if p, ok := reg.GetPlugin(name); ok {
			if setter, ok := p.(interface {
				SetOrchestrator(orch team.TeamOrchestrator)
				SetTemplateService(svc team.TeamTemplateService)
				SetMessageBus(bus messagebus.MessageBus)
			}); ok {
				setter.SetOrchestrator(orchestrator)
				setter.SetTemplateService(templateSvc)
				setter.SetMessageBus(bus)
				logger.Info("[Hivemind] injected Team dependencies into plugin %q", name)
			}
			if setter, ok := p.(interface {
				SetEventBridge(bridge *team.EventBridge)
			}); ok {
				setter.SetEventBridge(eventBridge)
				logger.Info("[Hivemind] injected EventBridge into plugin %q", name)
			}
		}
	}

	deps.TeamOrchestrator = orchestrator
	deps.TeamTemplateService = templateSvc
	deps.TeamMessageBus = bus
	logger.Info("[Hivemind] Team HTTP API dependencies ready")

	return nil
}

// setupTeamTranscript creates and registers the TranscriptPlugin.
func setupTeamTranscript(pf *plugin.Framework, bus messagebus.MessageBus) {
	reg := pf.Registry()
	for _, name := range reg.PluginNames() {
		if p, ok := reg.GetPlugin(name); ok {
			if provider, ok := p.(interface {
				TranscriptConfig() (enabled bool, outputDir string)
			}); ok {
				enabled, outputDir := provider.TranscriptConfig()
				if !enabled {
					logger.Info("[Hivemind] team transcript disabled by config")
					return
				}
				transcript := messagebus.NewTranscriptPlugin(messagebus.TranscriptConfig{
					Enabled:   true,
					OutputDir: outputDir,
					Format:    "markdown",
				})
				if hookable, ok := bus.(messagebus.HookRegistrar); ok {
					hookable.RegisterHook(messagebus.HookMessageSent, transcript.OnMessageSent)
					hookable.RegisterHook(messagebus.HookMessageBroadcast, transcript.OnMessageBroadcast)
					logger.Info("[Hivemind] team transcript enabled (output_dir=%s)", outputDir)
				}
				return
			}
		}
	}
}

// ============================================================
// Section 6: Channel Gateway
// ============================================================

// initChannelGateway creates and injects the ChannelManager.
func initChannelGateway(deps *Dependencies) error {
	if deps.Plugin == nil || deps.Agents == nil {
		return nil
	}

	gatewayCfg := DefaultGatewayConfig()
	dispatcher := gateway.NewDispatcher(deps.Agents.Service, nil, gatewayCfg.Defaults.AgentID)
	manager := gateway.NewChannelManager(dispatcher)
	dispatcher.SetChannelManager(manager)

	// Create Deliverer for trigger responses and inject into AgentRunner.
	// This enables sub-agent completion responses to be delivered to IM channels.
	deliverer := gateway.NewDeliverer(manager)
	deps.Agents.Runner.SetTriggerDeliverer(deliverer)
	logger.Info("[Hivemind] TriggerDeliverer injected into AgentRunner")

	// Inject into plugins
	reg := deps.Plugin.Registry()
	for _, name := range reg.PluginNames() {
		if p, ok := reg.GetPlugin(name); ok {
			if setter, ok := p.(interface {
				SetChannelManager(m *gateway.ChannelManager)
			}); ok {
				setter.SetChannelManager(manager)
				logger.Info("[Hivemind] injected ChannelManager into plugin %q", name)
			}
		}
	}

	deps.ChannelManager = manager
	return nil
}

// ============================================================
// Section 7: ModelManager Adapter
// ============================================================

// modelManagerAdapter bridges plugin.ModelManager to llm.ModelManager.
type modelManagerAdapter struct {
	llmManager llmService.ModelManager
}

var _ plugin.ModelManager = (*modelManagerAdapter)(nil)

func (m modelManagerAdapter) GetChatModel(ctx context.Context, providerID, modelID string) (model.BaseChatModel, error) {
	ref := llmEntity.ModelRef{ProviderID: providerID, ModelID: modelID}
	return m.llmManager.GetChatModel(ctx, ref)
}

func (m modelManagerAdapter) GetDefaultChatModel(ctx context.Context) (model.BaseChatModel, error) {
	return m.llmManager.GetDefaultChatModel(ctx)
}
