package node

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kiosk404/echoryn/pkg/logger"
	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
	"github.com/kiosk404/echoryn/pkg/skills"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Service manages the Golem node lifecycle: registration, heartbeat, and task reporting.
type Service struct {
	cfg *Config

	nodeID string
	conn   *grpc.ClientConn
	client pb.GolemNodeServiceClient

	mu      sync.RWMutex
	status  pb.NodeStatus
	cancel  context.CancelFunc
	stopped chan struct{}

	// taskHandler processes tasks dispatched from Hivemind via heartbeat stream.
	taskHandler TaskHandler

	// Runtime stats
	activeTasks int32
	queuedTasks int32
}

// NewService creates a new node Service from the given config.
func NewService(cfg *Config) (*Service, error) {
	return &Service{
		cfg:     cfg,
		nodeID:  uuid.New().String(),
		status:  pb.NodeStatus_NODE_STATUS_OFFLINE,
		stopped: make(chan struct{}),
	}, nil
}

// SetTaskHandler registers the handler for tasks dispatched via heartbeat stream.
func (s *Service) SetTaskHandler(handler TaskHandler) {
	s.taskHandler = handler
}

// NodeID returns the unique identifier of this Golem node.
func (s *Service) NodeID() string {
	return s.nodeID
}

// Status returns the current node status.
func (s *Service) Status() pb.NodeStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// SetStatus updates the node status.
func (s *Service) SetStatus(status pb.NodeStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
}

// Start connects to Hivemind, registers the node, and starts the heartbeat loop.
// Registration is handled by the heartbeat loop itself, which re-registers.
// before each heartbeat stream to handle Hivemind restarts gracefully.
func (s *Service) Start(ctx context.Context) error {
	// Connect to Hivemind gRPC.
	dialCtx, dialCancel := context.WithTimeout(ctx, s.cfg.ConnectTimeout)
	defer dialCancel()

	conn, err := grpc.DialContext(dialCtx, s.cfg.HivemindAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to Hivemind at %s: %w", s.cfg.HivemindAddress, err)
	}
	s.conn = conn
	s.client = pb.NewGolemNodeServiceClient(conn)
	logger.Info("[Golem] connected to Hivemind at %s", s.cfg.HivemindAddress)

	// Start heartbeat in background. (include registration).
	hbCtx, hbCancel := context.WithCancel(context.Background())
	s.cancel = hbCancel
	go s.heartbeatLoop(hbCtx)

	return nil
}

// Stop deregisters the node from Hivemind and closes the gRPC connection.
func (s *Service) Stop(ctx context.Context) error {
	// Cancel heartbeat loop.
	if s.cancel != nil {
		s.cancel()
	}

	// Wait for heartbeat to finish (with timeout).
	select {
	case <-s.stopped:
	case <-time.After(5 * time.Second):
		logger.Warn("[Golem] heartbeat loop did not stop in time")
	}

	// Deregister from Hivemind.
	if s.client != nil {
		_, err := s.client.Deregister(ctx, &pb.DeregisterRequest{
			NodeId: s.nodeID,
			Reason: "graceful shutdown",
		})
		if err != nil {
			logger.Warn("[Golem] deregister failed: %v", err)
		} else {
			logger.Info("[Golem] deregistered from Hivemind")
		}
	}

	// Close gRPC connection.
	if s.conn != nil {
		s.conn.Close()
	}

	s.SetStatus(pb.NodeStatus_NODE_STATUS_OFFLINE)
	return nil
}

// register sends the RegisterRequest to Hivemind.
func (s *Service) register(ctx context.Context) error {
	hostname, _ := os.Hostname()
	sysInfo := &pb.SystemInfo{
		CpuCores:   int32(runtime.NumCPU()),
		MemoryMb:   0, // TODO: detect system memory
		DiskFreeMb: 0, // TODO: detect disk space
		Os:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		Hostname:   hostname,
	}

	nodeInfo := &pb.NodeInfo{
		Id:           s.nodeID,
		Name:         s.cfg.NodeName,
		Status:       pb.NodeStatus_NODE_STATUS_ONLINE,
		SystemInfo:   sysInfo,
		Labels:       s.cfg.NodeLabels,
		RegisteredAt: timestamppb.Now(),
	}

	// Builtin capabilities always present.
	capMap := map[string]*pb.Capability{
		"shell":   {Name: "shell", Version: "1.0", Description: "Execute shell commands"},
		"fileops": {Name: "fileops", Version: "1.0", Description: "File operations (read/write/delete/search)"},
	}

	// Scan skills directory to discover installed skills and their capabilities.
	logger.Info("[Golem] skills scan: cfg.SkillsDir=%q", s.cfg.SkillsDir)
	if s.cfg.SkillsDir != "" {
		loader := skills.NewLoader(skills.WithGlobalSkillsDir(s.cfg.SkillsDir))
		logger.Info("[Golem] skills scan: loader globalDir=%s, projectDir=%s", loader.GlobalDir(), loader.ProjectDir())
		metadata, err := loader.LoadMetadataOnly(ctx)
		if err != nil {
			logger.Warn("[Golem] failed to load skills metadata: %v", err)
		} else {
			logger.Info("[Golem] skills scan: loaded %d metadata entries", len(metadata))
			for _, m := range metadata {
				logger.Info("[Golem] skills scan: adding skill %q (desc=%s, caps=%v, path=%s)",
					m.Name, m.Description[:min(len(m.Description), 50)], m.Capabilities, m.Path)
				nodeInfo.InstalledSkills = append(nodeInfo.InstalledSkills, &pb.InstalledSkill{
					Name:         m.Name,
					Description:  m.Description,
					Capabilities: m.Capabilities,
					Path:         m.Path,
				})
				// Merge skill-provided capabilities into the capability map.
				for _, capName := range m.Capabilities {
					if _, exists := capMap[capName]; !exists {
						capMap[capName] = &pb.Capability{
							Name:        capName,
							Version:     "1.0",
							Description: fmt.Sprintf("Provided by skill %q", m.Name),
						}
					}
				}
			}
			logger.Info("[Golem] discovered %d skill(s) from %s, nodeInfo.InstalledSkills=%d",
				len(metadata), s.cfg.SkillsDir, len(nodeInfo.InstalledSkills))
		}
	} else {
		logger.Warn("[Golem] skills scan: SkillsDir is empty, skipping skills discovery")
	}

	// Flatten capability map to slice.
	for _, cap := range capMap {
		nodeInfo.Capabilities = append(nodeInfo.Capabilities, cap)
	}

	loadInfo := s.collectLoadInfo()

	resp, err := s.client.Register(ctx, &pb.RegisterRequest{
		JoinToken: s.cfg.JoinToken,
		NodeInfo:  nodeInfo,
		LoadInfo:  loadInfo,
	})
	if err != nil {
		return fmt.Errorf("register RPC failed: %w", err)
	}

	if !resp.Accepted {
		return fmt.Errorf("registration rejected: %s", resp.RejectReason)
	}

	// Use the node ID assigned by Hivemind if provided.
	if resp.NodeId != "" {
		s.nodeID = resp.NodeId
	}

	s.SetStatus(pb.NodeStatus_NODE_STATUS_ONLINE)
	logger.Info("[Golem] registered successfully (nodeID=%s)", s.nodeID)
	return nil
}

// ReportTaskResult sends a task result to Hivemind.
func (s *Service) ReportTaskResult(ctx context.Context, result *pb.TaskResult) error {
	_, err := s.client.ReportTaskResult(ctx, &pb.ReportTaskResultRequest{
		NodeId:     s.nodeID,
		TaskResult: result,
	})
	return err
}

// ReportTaskProgress sends a task progress update to Hivemind.
func (s *Service) ReportTaskProgress(ctx context.Context, progress *pb.TaskProgress) error {
	_, err := s.client.ReportTaskProgress(ctx, &pb.ReportTaskProgressRequest{
		NodeId:       s.nodeID,
		TaskProgress: progress,
	})
	return err
}

// collectLoadInfo gathers current system load metrics.
func (s *Service) collectLoadInfo() *pb.NodeLoadInfo {
	s.mu.RLock()
	active := s.activeTasks
	queued := s.queuedTasks
	s.mu.RUnlock()

	return &pb.NodeLoadInfo{
		CpuPercent:    0, // TODO: collect real CPU usage
		MemoryPercent: 0, // TODO: collect real memory usage
		DiskFreeMb:    0, // TODO: collect real disk free
		ActiveTasks:   active,
		QueuedTasks:   queued,
	}
}

// IncrActiveTasks increments the active task count.
func (s *Service) IncrActiveTasks() {
	s.mu.Lock()
	s.activeTasks++
	s.mu.Unlock()
}

// DecrActiveTasks decrements the active task count.
func (s *Service) DecrActiveTasks() {
	s.mu.Lock()
	if s.activeTasks > 0 {
		s.activeTasks--
	}
	s.mu.Unlock()
}
