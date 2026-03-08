package golem

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/kiosk404/echoryn/internal/golem/config"
	"github.com/kiosk404/echoryn/internal/golem/handler"
	"github.com/kiosk404/echoryn/internal/golem/service/node"
	"github.com/kiosk404/echoryn/pkg/http/shutdown"
	"github.com/kiosk404/echoryn/pkg/http/shutdown/posixsignal"
	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/paths"
)

// golemServer holds all Golem components.
// In the stream-based architecture, Golem does NOT run a gRPC server.
// It only connects to Hivemind as a client and receives tasks via heartbeat stream.
type golemServer struct {
	gs           *shutdown.GracefulShutdown
	nodeService  *node.Service
	taskExecutor *handler.TaskExecutor
	cfg          *config.Config
}

type preparedGolemServer struct {
	*golemServer
}

func createGolemServer(cfg *config.Config) (*golemServer, error) {
	// Ensure Golem state directory structure.
	stateDir, err := paths.EnsureStateDirForRole(paths.RoleGolem)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure golem state directory: %w", err)
	}
	logger.Info("[Golem] state directory: %s", stateDir)

	gs := shutdown.New()
	gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

	// Resolve node name (default to hostname).
	nodeName := cfg.NodeName
	if nodeName == "" {
		hostname, _ := os.Hostname()
		nodeName = hostname
	}

	// Resolve workspace directory.
	workspaceDir := cfg.WorkspaceDir
	if workspaceDir == "" {
		workspaceDir = paths.ResolveGolemWorkspace()
	}

	// Resolve skills directory.
	skillsDir := cfg.SkillsDir
	if skillsDir == "" {
		skillsDir = paths.ResolveGolemSkillsDir()
	}

	// Parse time durations.
	heartbeatInterval, err := time.ParseDuration(cfg.HeartbeatInterval)
	if err != nil {
		heartbeatInterval = 15 * time.Second
	}
	connectTimeout, err := time.ParseDuration(cfg.ConnectTimeout)
	if err != nil {
		connectTimeout = 10 * time.Second
	}
	reconnectInterval, err := time.ParseDuration(cfg.ReconnectInterval)
	if err != nil {
		reconnectInterval = 5 * time.Second
	}

	// Initialize Node Service (handles registration, heartbeat, reporting).
	nodeCfg := &node.Config{
		NodeName:           nodeName,
		NodeLabels:         cfg.NodeLabels,
		HivemindAddress:    cfg.HivemindAddress,
		HeartbeatInterval:  heartbeatInterval,
		ConnectTimeout:     connectTimeout,
		ReconnectInterval:  reconnectInterval,
		MaxConcurrentTasks: int32(cfg.MaxConcurrentTasks),
		WorkspaceDir:       workspaceDir,
		SkillsDir:          skillsDir,
		JoinToken:          cfg.JoinToken,
	}
	nodeService, err := nodeCfg.Complete().New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize node service: %w", err)
	}
	logger.Info("[Golem] node service initialized (name=%s, hivemind=%s)", nodeName, cfg.HivemindAddress)

	// Create Task Executor (handles tasks dispatched via heartbeat stream).
	taskExecutor := handler.NewTaskExecutor(nodeService)
	nodeService.SetTaskHandler(taskExecutor)
	logger.Info("[Golem] task executor registered (stream-based, no local gRPC server)")

	server := &golemServer{
		gs:           gs,
		nodeService:  nodeService,
		taskExecutor: taskExecutor,
		cfg:          cfg,
	}

	return server, nil
}

func (s *golemServer) PrepareRun() preparedGolemServer {
	// Register shutdown callbacks.
	s.gs.AddShutdownCallback(shutdown.Func(func(string) error {
		logger.Info("[Golem] shutting down...")

		// Stop node service (deregister from Hivemind, stop heartbeat).
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.nodeService.Stop(ctx); err != nil {
			logger.Warn("[Golem] node service stop error: %v", err)
		}

		logger.Info("[Golem] shutdown complete")
		return nil
	}))

	return preparedGolemServer{s}
}

func (s preparedGolemServer) Run() error {
	// Start node service (connect to Hivemind, register, start heartbeat).
	// Tasks are received via the heartbeat bidirectional stream.
	ctx := context.Background()
	if err := s.nodeService.Start(ctx); err != nil {
		return fmt.Errorf("failed to start node service: %w", err)
	}

	// Start shutdown manager (blocks until signal received).
	if err := s.gs.Start(); err != nil {
		log.Fatalf("start shutdown manager failed: %s", err.Error())
	}

	// Block forever (shutdown manager handles exit).
	select {}
}
