package hivemind

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/config"
	grpchandler "github.com/kiosk404/echoryn/internal/hivemind/handler/grpc"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents"
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
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin/builtin"
	genericapiserver "github.com/kiosk404/echoryn/internal/pkg/server"
	"github.com/kiosk404/echoryn/pkg/http/shutdown"
	"github.com/kiosk404/echoryn/pkg/http/shutdown/posixsignal"
	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/paths"
	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type apiServer struct {
	gs               *shutdown.GracefulShutdown
	gRPCAPIServer    *genericapiserver.GRPCAPIServer
	genericAPIServer *genericapiserver.GenericAPIServer

	pluginFramework *plugin.Framework
	llmModule       *llm.Module
	mcpModule       *mcp.Module
	agentsModule    *agents.Module
	channelManager  *gateway.ChannelManager
	golemModule     *golem.Module
}

type preparedAPIServer struct {
	*apiServer
}

// ExtraConfig defines extra configuration for the API server.
type ExtraConfig struct {
	Addr       string
	MaxMsgSize int
}

type completedExtraConfig struct {
	*ExtraConfig
}

// Complete fills in any fields not set that are required to have valid data and can be derived from other fields.
func (c *ExtraConfig) complete() *completedExtraConfig {
	if c.Addr == "" {
		c.Addr = "127.0.0.1:11788"
	}

	return &completedExtraConfig{c}
}

// New create a grpcAPIServer instance.
func (c *completedExtraConfig) New(extraOpts ...grpc.ServerOption) (*genericapiserver.GRPCAPIServer, error) {
	opts := []grpc.ServerOption{grpc.MaxRecvMsgSize(c.MaxMsgSize)}
	opts = append(opts, extraOpts...)
	grpcServer := grpc.NewServer(opts...)

	reflection.Register(grpcServer)

	return genericapiserver.NewGRPCAPIServer(grpcServer, c.Addr), nil
}

func createAPIServer(cfg *config.Config) (*apiServer, error) {
	// Ensure ~/.echoryn state directory structure exists.
	stateDir, err := paths.EnsureStateDir()
	if err != nil {
		return nil, fmt.Errorf("failed to ensure state directory: %w", err)
	}
	logger.Info("[Hivemind] state directory: %s", stateDir)

	gs := shutdown.New()
	gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

	genericConfig, err := buildGenericConfig(cfg)
	if err != nil {
		return nil, err
	}

	extraConfig, err := buildExtraConfig(cfg)
	if err != nil {
		return nil, err
	}

	genericServer, err := genericConfig.Complete().New()
	if err != nil {
		return nil, err
	}

	// Initialize Golem subsystem (registry + dispatcher + scheduler + tokenmanager)
	// Must be created before gRPC server so the auth interceptor can reference TokenManager.
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
		return nil, fmt.Errorf("failed to initialize Golem subsystem: %w", err)
	}
	if err := golemModule.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to start Golem subsystem: %w", err)
	}
	logger.Info("[Hivemind] Golem subsystem initialized successfully (admin_token available)")

	// Persist admin token to credentials file for easy access.
	adminTokenPath := paths.ResolveAdminTokenPath()
	if err := os.WriteFile(adminTokenPath, []byte(golemModule.TokenManager.AdminToken()), 0o600); err != nil {
		logger.Warn("[Hivemind] failed to write admin token to %s:%v", adminTokenPath, err)
	} else {
		logger.Info("[Hivemind] Admin token saved to: %s", adminTokenPath)
	}

	// Create gRPC server with Admin Auth interceptors.
	extraServer, err := extraConfig.complete().New(
		grpc.ChainUnaryInterceptor(grpchandler.AdminAuthUnaryInterceptor(golemModule.TokenManager)),
		grpc.ChainStreamInterceptor(grpchandler.AdminAuthStreamInterceptor(golemModule.TokenManager)),
	)
	if err != nil {
		return nil, err
	}

	// Initialize LLM module
	llmCfg := &llm.Config{
		ModelOptions: cfg.ModelOptions,
	}
	llmModule, err := llmCfg.Complete().New(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LLM module: %w", err)
	}
	logger.Info("LLM module initialized successfully")

	// Register Golem gRPC services on the gRPC server
	devMode := cfg.GolemOptions != nil && cfg.GolemOptions.DevMode
	nodeServiceHandler := grpchandler.NewNodeServiceHandler(golemModule.Registry, golemModule.TokenManager, devMode)
	adminServiceHandler := grpchandler.NewAdminServiceHandler(golemModule.Registry, golemModule.TokenManager)
	pb.RegisterGolemNodeServiceServer(extraServer.Server, nodeServiceHandler)
	pb.RegisterHivemindAdminServiceServer(extraServer.Server, adminServiceHandler)

	// Bind the NodeServiceHandler as the StreamManager for the dispatcher.
	// This enables task dispatch through the heartbeat bidirectional stream.
	golemModule.BindStreamManager(nodeServiceHandler)
	logger.Info("[Hivemind] Golem gRPC services registered (GolemNodeService + HivemindAdminService + stream-based dispatch)")
	if devMode {
		logger.Warn("[Hivemind] Golem dev-mode ENABLED: loopback nodes can register without join token")
	}

	pluginCfg := &plugin.Config{
		SlotConfig: plugin.SlotConfig{
			"memory": cfg.PluginOptions.Slots.Memory,
		},
		RuntimeAPI: plugin.NewRuntimeAPI(&modelManagerAdapter{llmModule.Manager}),
	}
	pluginFramework := pluginCfg.Complete().New()

	// PromptPipeline is always created (builtin sections work without plugins).
	promptPipeline := prompt.NewDefaultPipeline()
	pluginFramework.SetPromptPipeline(promptPipeline)

	if cfg.PluginOptions.Enabled {
		// Register in-tree (built-in) plugins.
		// All plugin configurations are sourced from PluginOptions.Entries,
		// following OpenClaw's plugins.entries[pluginID].config pattern.
		inTreeRegistry := builtin.NewInTreeRegistry(cfg.PluginOptions, golemModule)
		if err := inTreeRegistry.ApplyTo(pluginFramework); err != nil {
			return nil, fmt.Errorf("failed to register in-tree plugins: %w", err)
		}

		// Initialize all plugins (slot resolution → factory → Init).
		if err := pluginFramework.Init(); err != nil {
			return nil, fmt.Errorf("failed to initialize plugin framework: %w", err)
		}

		// Start plugin lifecycle (services, hooks).
		if err := pluginFramework.Start(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to start plugin framework: %w", err)
		}
		logger.Info("[Hivemind] Plugin framework initialized successfully (%d plugins loaded)",
			pluginFramework.Registry().Len())

		// Bind SkillsEnricher: probe initialized plugins for the registry.SkillsEnricher
		// interface and inject it into the Golem Registry. This allows the Registry to
		// enrich Golem node registrations with Hivemind-side skills even when nodes
		// don't report their own skills (e.g. no local skills directory).
		injectSkillsEnricher(pluginFramework, golemModule)
	} else {
		logger.Info("[Hivemind] Plugin framework disabled (plugins.enabled=false), skipping plugin loading")
	}

	// Initialize MCP module (K8S-style: Config → Complete → New).
	// Load MCP configuration from standalone file (Claude Desktop compatible format).
	mcpFileCfg, err := mcp.LoadMCPConfig(cfg.MCPOptions.ConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load MCP config from %q: %w", cfg.MCPOptions.ConfigFile, err)
	}
	mcpCfg := &mcp.Config{
		MCPConfig: mcpFileCfg,
	}
	mcpModule, err := mcpCfg.Complete().New(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP module: %w", err)
	}
	logger.Info("[Hivemind] MCP module initialized successfully")

	// Initialize Agents module (K8S-style: Config → Complete → New).
	agentsCfg := &agents.Config{}
	agentsModule, err := agentsCfg.Complete().New(context.Background(), agents.Dependencies{
		LLM:     llmModule,
		Plugins: pluginFramework,
		MCP:     mcpModule.Manager,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Agents module: %w", err)
	}
	logger.Info("[Hivemind] Agents module initialized successfully")
	logger.Info("[Hivemind] Pipeline status: sections=%d mutators=%d",
		promptPipeline.SectionCount(), promptPipeline.MutatorCount())

	server := &apiServer{
		gs:               gs,
		genericAPIServer: genericServer,
		gRPCAPIServer:    extraServer,
		llmModule:        llmModule,
		pluginFramework:  pluginFramework,
		mcpModule:        mcpModule,
		agentsModule:     agentsModule,
		golemModule:      golemModule,
	}

	// Initialize IM channel gateway.
	// Create ChannelManager + Dispatcher, then inject the manager into
	// channel plugins via k8s-style interface probe (ChannelManagerSetter).
	server.initChannelGateway()

	return server, nil
}

func (s *apiServer) PrepareRun() preparedAPIServer {
	gatewayCfg := DefaultGatewayConfig()

	initRouter(s.genericAPIServer.Engine, &routerDeps{
		agentService:  s.agentsModule.Service,
		llmManager:    s.llmModule.Manager,
		authConfig:    &gatewayCfg.Auth,
		gatewayConfig: gatewayCfg,
	})

	// Start IM channel gateway if ChannelManager is initialized.
	if s.channelManager != nil {
		if err := s.channelManager.StartAll(context.Background()); err != nil {
			logger.Warn("[Hivemind] channel gateway start errors: %v", err)
		} else {
			logger.Info("[Hivemind] IM channel gateway started")
		}
	}

	s.gs.AddShutdownCallback(shutdown.Func(func(string) error {
		// Stop IM channel gateway first.
		if s.channelManager != nil {
			s.channelManager.StopAll(context.Background())
		}
		// Stop Plugin framework (reverse lifecycle: hooks -> services -> plugins).
		if s.pluginFramework != nil {
			s.pluginFramework.Stop(context.Background())
		}
		// Close MCP module (disconnect all MCP servers)
		if s.mcpModule != nil {
			s.mcpModule.Close()
		}
		// Close agent module(BoltDB handle if any)
		if s.agentsModule != nil {
			s.agentsModule.Close()
		}
		// Stop Golem subsystem (registry + dispatcher + scheduler)
		if s.golemModule != nil {
			s.golemModule.Stop(context.Background())
		}
		s.gRPCAPIServer.Stop()
		s.genericAPIServer.Close()
		return nil
	}))
	return preparedAPIServer{s}
}

func (s preparedAPIServer) Run() error {
	go s.gRPCAPIServer.Run()

	// start shutdown managers
	if err := s.gs.Start(); err != nil {
		log.Fatalf("start shutdown manager failed: %s", err.Error())
	}

	return s.genericAPIServer.Run()
}

func buildGenericConfig(cfg *config.Config) (genericConfig *genericapiserver.Config, lastErr error) {
	genericConfig = genericapiserver.NewConfig()
	if lastErr = cfg.GenericServerRunOptions.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	return
}

func buildExtraConfig(cfg *config.Config) (*ExtraConfig, error) {
	return &ExtraConfig{
		Addr:       fmt.Sprintf("%s:%d", cfg.GRPCOptions.BindAddress, cfg.GRPCOptions.BindPort),
		MaxMsgSize: cfg.GRPCOptions.MaxMsgSize,
	}, nil
}

// --- Skills Enricher injection ---

// injectSkillsEnricher probes all initialized plugins for the registry.SkillsEnricher
// interface and binds the first match to the Golem Registry.
// injectSkillsEnricher probes all initialized plugins for the registry.SkillsEnricher
// interface and binds the first match to the Golem Registry.
func injectSkillsEnricher(pf *plugin.Framework, gm *golem.Module) {
	if pf == nil || gm == nil {
		return
	}

	reg := pf.Registry()
	for _, name := range reg.PluginNames() {
		p, ok := reg.GetPlugin(name)
		if !ok {
			continue
		}
		if enricher, ok := p.(registry.SkillsEnricher); ok {
			gm.Registry.SetSkillsEnricher(enricher)
			globalSkills := enricher.GetGlobalSkills()
			logger.Info("[Hivemind] injected SkillsEnricher from plugin %q into Golem Registry (%d global skills)",
				name, len(globalSkills))
			return
		}
	}
	logger.Debug("[Hivemind] no SkillsEnricher found among initialized plugins")
}

// --- IM Channel Gateway ---

// channelManagerSetter is the interface that channel plugins implement
// to receive the shared ChannelManager via k8s-style interface probe.
type channelManagerSetter interface {
	SetChannelManager(m *gateway.ChannelManager)
}

func (s *apiServer) initChannelGateway() {
	if s.pluginFramework == nil || s.agentsModule == nil {
		return
	}

	gatewayCfg := DefaultGatewayConfig()
	defaultAgentID := gatewayCfg.Defaults.AgentID

	// Create Dispatcher (implements InboundHandler)
	dispatcher := gateway.NewDispatcher(s.agentsModule.Service, nil, defaultAgentID)

	// Create ChannelManager with the dispatcher as the inbound handler.
	manager := gateway.NewChannelManager(dispatcher)

	// Wire the dispatcher's channel manager reference.
	dispatcher.SetChannelManager(manager)

	// Inject ChannelManager into all channel plugins via interface probe.
	registry := s.pluginFramework.Registry()
	for _, name := range registry.PluginNames() {
		p, ok := registry.GetPlugin(name)
		if !ok {
			continue
		}
		if setter, ok := p.(channelManagerSetter); ok {
			setter.SetChannelManager(manager)
			logger.Info("[Hivemind] injected ChannelManager into plugin %q", name)
		}
	}

	// Store the manager so PrepareRun can StartAll and shutdown can StopAll.
	s.channelManager = manager
}

// --- ModelManager Adapter ---
// Bridge between plugin.ModelManager (string-based) and llm.ModelManager (entity-based)
type modelManagerAdapter struct {
	llmManager llmService.ModelManager
}

var _ plugin.ModelManager = (*modelManagerAdapter)(nil)

func (m modelManagerAdapter) GetChatModel(ctx context.Context, provideID, modelID string) (model.BaseChatModel, error) {
	ref := llmEntity.ModelRef{ProviderID: provideID, ModelID: modelID}
	return m.llmManager.GetChatModel(ctx, ref)
}

func (m modelManagerAdapter) GetDefaultChatModel(ctx context.Context) (model.BaseChatModel, error) {
	return m.llmManager.GetDefaultChatModel(ctx)
}
