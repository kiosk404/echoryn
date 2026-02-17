package hivemind

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/components/model"
	"github.com/kiosk404/echoryn/internal/hivemind/config"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents/domain/service/runtime/prompt"
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
func (c *completedExtraConfig) New() (*genericapiserver.GRPCAPIServer, error) {
	opts := []grpc.ServerOption{grpc.MaxRecvMsgSize(c.MaxMsgSize)}
	grpcServer := grpc.NewServer(opts...)

	reflection.Register(grpcServer)

	return genericapiserver.NewGRPCAPIServer(grpcServer, c.Addr), nil
}

func createAPIServer(cfg *config.Config) (*apiServer, error) {
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
	extraServer, err := extraConfig.complete().New()
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
		inTreeRegistry := builtin.NewInTreeRegistry(cfg.PluginOptions)
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

	server := &apiServer{
		gs:               gs,
		genericAPIServer: genericServer,
		gRPCAPIServer:    extraServer,
		llmModule:        llmModule,
		pluginFramework:  pluginFramework,
		mcpModule:        mcpModule,
		agentsModule:     agentsModule,
	}

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

	s.gs.AddShutdownCallback(shutdown.Func(func(string) error {
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
