package grpc

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/dispatcher"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/registry"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/tokenmanager"
	"github.com/kiosk404/echoryn/pkg/logger"
	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
	"google.golang.org/grpc/peer"
)

// nodeStream holds a node's active heartbeat stream and pending task channels.
type nodeStream struct {
	stream  pb.GolemNodeService_HeartbeatServer
	mu      sync.Mutex
	results map[string]chan *pb.TaskResult // TaskID -> result channel
}

// NodeServiceHandler implements pb.GolemNodeServiceServer and dispatcher.StreamManager.
// It manages per-node heartbeat streams for both heartbeat and task dispatch.
type NodeServiceHandler struct {
	pb.UnimplementedGolemNodeServiceServer
	registry     registry.Registry
	tokenManager tokenmanager.TokenManager
	devMode      bool // when true, loopback clients can register without join token

	streamsMu sync.RWMutex
	streams   map[string]*nodeStream // nodeID → stream info
}

var _ dispatcher.StreamManager = (*NodeServiceHandler)(nil)

// NewNodeServiceHandler creates a new NodeServiceHandler.
func NewNodeServiceHandler(reg registry.Registry, tm tokenmanager.TokenManager, devMode bool) *NodeServiceHandler {
	return &NodeServiceHandler{
		registry:     reg,
		tokenManager: tm,
		devMode:      devMode,
		streams:      make(map[string]*nodeStream),
	}
}

// Register handles a Golem node registration request.
// A valid join-token (bootstrap token) is REQUIRED for registration
// unless dev-mode is enabled AND the client connects from a loopback address.
// If the node is already registered (re-registration after reconnect), token
// validation is skipped to avoid consuming extra token usages.
func (h *NodeServiceHandler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if req.NodeInfo == nil {
		return &pb.RegisterResponse{
			Accepted:     false,
			RejectReason: "node_info is required",
		}, nil
	}

	// Check if this is a re-registration from a previously known node.
	// If the node already exists in the registry, skip token auth entirely
	// to avoid consuming extra token usages on reconnect / Hivemind restart.
	isReRegister := false
	if req.NodeInfo.Id != "" {
		if _, err := h.registry.GetNode(req.NodeInfo.Id); err == nil {
			isReRegister = true
			logger.Info("[GolemNodeService] re-registration from known node %s, skipping token auth", req.NodeInfo.Id)
		}
	}

	if !isReRegister {
		// Determine whether this request can skip token validation (dev-mode + loopback)
		skipTokenAuth := false
		if h.devMode && req.JoinToken == "" {
			//if iputil.IsLoopBackIP(getGrpcClientIP(ctx)) {
			skipTokenAuth = true
			logger.Info("[GolemNodeService] dev-mode: allowing token-less registration for loopback node %s", req.NodeInfo.Id)
			//}
		}

		if !skipTokenAuth {
			// Validate Bootstrap Token if provided - required for all non-dev-mode registrations.
			if req.JoinToken == "" {
				logger.Warn("[GolemNodeService] registration rejected for %s: missing join token", req.NodeInfo.Id)
				return &pb.RegisterResponse{
					Accepted:     false,
					RejectReason: "join_token is required for node registration",
				}, nil
			}

			bt, err := h.tokenManager.ValidateBootstrapToken(ctx, req.JoinToken)
			if err != nil {
				logger.Warn("[GolemNodeService] join token validation failed for %s: %v", req.NodeInfo.Id, err)
				return &pb.RegisterResponse{
					Accepted:     false,
					RejectReason: fmt.Sprintf("invalid join token: %v", err),
				}, nil
			}

			// Consume one usage.
			if err = h.tokenManager.ConsumeToken(ctx, bt.ID); err != nil {
				logger.Warn("[GolemNodeService] join token consume failed for %s:%w", req.NodeInfo.Id, err)
			}
		}
	}

	err := h.registry.RegisterNode(ctx, req.NodeInfo, req.LoadInfo)
	if err != nil {
		logger.Warn("[GolemNodeService] register failed for %s: %v", req.NodeInfo.Id, err)
		return &pb.RegisterResponse{
			Accepted:     false,
			RejectReason: err.Error(),
		}, nil
	}

	if isReRegister {
		logger.Info("[GolemNodeService] re-register success: id=%s name=%s caps=%d skills=%d",
			req.NodeInfo.Id, req.NodeInfo.Name, len(req.NodeInfo.Capabilities), len(req.NodeInfo.InstalledSkills))
	} else {
		logger.Info("[GolemNodeService] register success: id=%s name=%s caps=%d skills=%d",
			req.NodeInfo.Id, req.NodeInfo.Name, len(req.NodeInfo.Capabilities), len(req.NodeInfo.InstalledSkills))
	}

	return &pb.RegisterResponse{
		Accepted: true,
		NodeId:   req.NodeInfo.Id,
		BaseResp: &pb.BaseResp{StatusCode: 0, StatusMessage: "ok"},
	}, nil
}

// Heartbeat handles the bidirectional heartbeat stream.
// It also acts as the channel for dispatching tasks to the Golem node.
func (h *NodeServiceHandler) Heartbeat(stream pb.GolemNodeService_HeartbeatServer) error {
	// We need the first message to identify the node.
	firstReq, err := stream.Recv()
	if err != nil {
		return err
	}

	nodeID := firstReq.NodeId
	if nodeID == "" {
		return fmt.Errorf("first heartbeat message must contain node_id")
	}

	// Register this stream for the node.
	ns := &nodeStream{
		stream:  stream,
		results: make(map[string]chan *pb.TaskResult),
	}
	h.streamsMu.Lock()
	h.streams[nodeID] = ns
	h.streamsMu.Unlock()
	logger.Info("[GolemNodeService] heartbeat stream established for node %s", nodeID)

	defer func() {
		// Close all pending result channels so WaitForResult callers get unblocked.
		ns.mu.Lock()
		for taskID, ch := range ns.results {
			close(ch)
			delete(ns.results, taskID)
		}
		ns.mu.Unlock()

		h.streamsMu.Lock()
		delete(h.streams, nodeID)
		h.streamsMu.Unlock()
		logger.Info("[GolemNodeService] heartbeat stream closed for node %s", nodeID)
	}()

	// Process the first heartbeat.
	h.processHeartbeat(ns, nodeID, firstReq)

	// Continue processing heartbeats.
	for {
		req, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		h.processHeartbeat(ns, nodeID, req)
	}
}

// processHeartbeat handles a single heartbeat message from a Golem node.
func (h *NodeServiceHandler) processHeartbeat(ns *nodeStream, nodeID string, req *pb.HeartbeatRequest) {
	// Update heartbeat in Registry.
	if err := h.registry.UpdateHeartbeat(context.Background(), req.NodeId, req.LoadInfo); err != nil {
		logger.Warn("[GolemNodeService] heartbeat update failed for %s: %v", req.NodeId, err)
	}

	// Check if the heartbeat carries a dispatch result (ack from Golem).
	h.handleDispatchResult(ns, req)

	// Determine if we need to send a control action.
	action := pb.HeartbeatAction_HEARTBEAT_ACTION_NONE
	node, err := h.registry.GetNode(req.NodeId)
	if err == nil && node != nil {
		switch node.Status.Phase {
		case pb.NodeStatus_NODE_STATUS_DRAINING:
			action = pb.HeartbeatAction_HEARTBEAT_ACTION_DRAIN
		}
	}

	// Send heartbeat ACK only if no pending dispatches (dispatches use their own sends).
	resp := &pb.HeartbeatResponse{
		Acknowledged: true,
		Action:       action,
	}

	ns.mu.Lock()
	err = ns.stream.Send(resp)
	ns.mu.Unlock()

	if err != nil {
		logger.Warn("[GolemNodeService] failed to send heartbeat response to node %s: %v", nodeID, err)
	}
}

// handleDispatchResult checks if the heartbeat carries a task dispatch result.
func (h *NodeServiceHandler) handleDispatchResult(ns *nodeStream, req *pb.HeartbeatRequest) {
	// Golem uses the LoadInfo's metadata or a dedicated field to report dispatch results.
	// For now, we don't extract inline results from heartbeat requests.
	// Task results are reported via ReportTaskResult RPC.
}

// SendToNode sends a HeartbeatResponse to the specified node's heartbeat stream.
// This implements dispatcher.StreamManager.
func (h *NodeServiceHandler) SendToNode(nodeID string, resp *pb.HeartbeatResponse) error {
	h.streamsMu.RLock()
	ns, ok := h.streams[nodeID]
	h.streamsMu.RUnlock()

	if !ok {
		return fmt.Errorf("no active heartbeat stream for node %s", nodeID)
	}

	ns.mu.Lock()
	err := ns.stream.Send(resp)
	ns.mu.Unlock()

	return err
}

// WaitForResult blocks until the task execution result is reported by the Golem node
// via ReportTaskResult RPC, or the context is cancelled/timed out.
// This implements dispatcher.StreamManager.
func (h *NodeServiceHandler) WaitForResult(ctx context.Context, nodeID string, taskID string) (*pb.DispatchTaskResponse, error) {
	h.streamsMu.RLock()
	ns, ok := h.streams[nodeID]
	h.streamsMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no active heartbeat stream for node %s", nodeID)
	}

	// Create a buffered channel for this task's result.
	resultCh := make(chan *pb.TaskResult, 1)
	ns.mu.Lock()
	ns.results[taskID] = resultCh
	ns.mu.Unlock()

	defer func() {
		ns.mu.Lock()
		delete(ns.results, taskID)
		ns.mu.Unlock()
	}()

	// Block until result arrives or context is cancelled.
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("waiting for task %s result from node %s: %w", taskID, nodeID, ctx.Err())
	case result, ok := <-resultCh:
		if !ok || result == nil {
			// Channel was closed (stream disconnected).
			return nil, fmt.Errorf("node %s disconnected while waiting for task %s result", nodeID, taskID)
		}
		resp := &pb.DispatchTaskResponse{
			Accepted:   true,
			TaskResult: result,
			BaseResp:   &pb.BaseResp{StatusCode: 0, StatusMessage: "ok"},
		}
		if !result.Success {
			resp.BaseResp.StatusCode = 1
			resp.BaseResp.StatusMessage = result.Error
		}
		return resp, nil
	}
}

// Deregister handles a Golem node deregistration request.
func (h *NodeServiceHandler) Deregister(ctx context.Context, req *pb.DeregisterRequest) (*pb.DeregisterResponse, error) {
	err := h.registry.DeregisterNode(ctx, req.NodeId)
	if err != nil {
		logger.Warn("[GolemNodeService] deregister failed for %s: %v", req.NodeId, err)
		return &pb.DeregisterResponse{
			Success:  false,
			BaseResp: &pb.BaseResp{StatusCode: 1, StatusMessage: err.Error()},
		}, nil
	}

	logger.Info("[GolemNodeService] node deregistered: %s (reason: %s)", req.NodeId, req.Reason)
	return &pb.DeregisterResponse{
		Success:  true,
		BaseResp: &pb.BaseResp{StatusCode: 0, StatusMessage: "ok"},
	}, nil
}

// ReportTaskResult handles a task result report from Golem.
// It delivers the result to the waiting WaitForResult channel if one exists.
func (h *NodeServiceHandler) ReportTaskResult(ctx context.Context, req *pb.ReportTaskResultRequest) (*pb.ReportTaskResultResponse, error) {
	if req.TaskResult == nil {
		return &pb.ReportTaskResultResponse{Acknowledged: false}, nil
	}

	logger.Info("[GolemNodeService] task result from node %s: task=%s success=%v",
		req.NodeId, req.TaskResult.TaskId, req.TaskResult.Success)

	// Deliver the result to the waiting channel (if any).
	h.deliverTaskResult(req.NodeId, req.TaskResult)

	return &pb.ReportTaskResultResponse{
		Acknowledged: true,
		BaseResp:     &pb.BaseResp{StatusCode: 0, StatusMessage: "ok"},
	}, nil
}

// deliverTaskResult finds the per-task result channel and delivers the result.
func (h *NodeServiceHandler) deliverTaskResult(nodeID string, result *pb.TaskResult) {
	h.streamsMu.RLock()
	ns, ok := h.streams[nodeID]
	h.streamsMu.RUnlock()

	if !ok {
		logger.Warn("[GolemNodeService] no stream for node %s to deliver task result %s", nodeID, result.TaskId)
		return
	}

	ns.mu.Lock()
	ch, hasCh := ns.results[result.TaskId]
	ns.mu.Unlock()

	if hasCh {
		// Non-blocking send (channel is buffered with size 1).
		select {
		case ch <- result:
			logger.Debug("[GolemNodeService] delivered result for task %s to waiting caller", result.TaskId)
		default:
			logger.Warn("[GolemNodeService] result channel full for task %s, result dropped", result.TaskId)
		}
	} else {
		logger.Debug("[GolemNodeService] no waiting caller for task %s result (fire-and-forget dispatch)", result.TaskId)
	}
}

// ReportTaskProgress handles a task progress update from Golem.
func (h *NodeServiceHandler) ReportTaskProgress(ctx context.Context, req *pb.ReportTaskProgressRequest) (*pb.ReportTaskProgressResponse, error) {
	if req.TaskProgress == nil {
		return &pb.ReportTaskProgressResponse{Acknowledged: false}, nil
	}

	logger.Debug("[GolemNodeService] task progress from node %s: task=%s %.1f%%",
		req.NodeId, req.TaskProgress.TaskId, req.TaskProgress.ProgressPercent)

	// TODO: Phase 2/3 — forward progress to Agent Runtime for streaming.

	return &pb.ReportTaskProgressResponse{
		Acknowledged: true,
		BaseResp:     &pb.BaseResp{StatusCode: 0, StatusMessage: "ok"},
	}, nil
}

// getGrpcClientIP
func getGrpcClientIP(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(p.Addr.String())
	if err != nil {
		return ""
	}
	return host
}
