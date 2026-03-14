package hivemind

import (
	"context"
	"fmt"
	"log"

	"github.com/kiosk404/echoryn/internal/hivemind/config"
	"github.com/kiosk404/echoryn/internal/hivemind/service/agents"
	"github.com/kiosk404/echoryn/internal/hivemind/service/gateway"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem"
	"github.com/kiosk404/echoryn/internal/hivemind/service/llm"
	"github.com/kiosk404/echoryn/internal/hivemind/service/mcp"
	"github.com/kiosk404/echoryn/internal/hivemind/service/messagebus"
	"github.com/kiosk404/echoryn/internal/hivemind/service/plugin"
	"github.com/kiosk404/echoryn/internal/hivemind/service/team"
	genericapiserver "github.com/kiosk404/echoryn/internal/pkg/server"
	"github.com/kiosk404/echoryn/pkg/http/shutdown"
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
	channelManager  *gateway.ChannelManager
	golemModule     *golem.Module

	// Team subsystem (populated by injectTeamDependencies).
	teamOrchestrator    team.TeamOrchestrator
	teamTemplateService team.TeamTemplateService
	teamMessageBus      messagebus.MessageBus
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
	deps := &Dependencies{Config: cfg}

	chain := InitializerChain{
		InitInfrastructure(cfg),
		InitGolem(),
		InitLLM(),
		InitMCP(),
		InitAgents(),
		InitPluginLifecycle(),
	}

	if err := chain.Run(context.Background(), deps); err != nil {
		return nil, err
	}
	return &apiServer{
		gs:                  deps.Shutdown,
		genericAPIServer:    deps.GenericServer,
		gRPCAPIServer:       deps.GRPCServer,
		llmModule:           deps.LLM,
		pluginFramework:     deps.Plugin,
		mcpModule:           deps.MCP,
		agentsModule:        deps.Agents,
		golemModule:         deps.Golem,
		channelManager:      deps.ChannelManager,
		teamOrchestrator:    deps.TeamOrchestrator,
		teamTemplateService: deps.TeamTemplateService,
		teamMessageBus:      deps.TeamMessageBus,
	}, nil
}

func (s *apiServer) PrepareRun() preparedAPIServer {
	gatewayCfg := DefaultGatewayConfig()

	initRouter(s.genericAPIServer.Engine, &routerDeps{
		agentService:        s.agentsModule.Service,
		llmManager:          s.llmModule.Manager,
		authConfig:          &gatewayCfg.Auth,
		gatewayConfig:       gatewayCfg,
		teamOrchestrator:    s.teamOrchestrator,
		teamTemplateService: s.teamTemplateService,
		teamMessageBus:      s.teamMessageBus,
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
