package node

import (
	"context"
	"io"
	"time"

	"github.com/kiosk404/echoryn/pkg/logger"
	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TaskHandler handles tasks dispatched from Hivemind via the heartbeat stream.
type TaskHandler interface {
	// HandleTask executes a dispatched task asynchronously.
	HandleTask(ctx context.Context, task *pb.Task)
	// CancelTask cancels a running task.
	CancelTask(taskID string, reason string)
}

// heartbeatLoop maintains a bidirectional heartbeat stream with Hivemind.
// It reconnects automatically on stream failures.
// On reconnect, it re-registers the node first to handle Hivemind restarts
// where the in-memory registry is cleared.
// The heartbeat stream is now the sole channel for receiving tasks from Hivemind.
func (s *Service) heartbeatLoop(ctx context.Context) {
	defer close(s.stopped)

	for {
		select {
		case <-ctx.Done():
			logger.Info("[Golem] heartbeat loop stopped")
			return
		default:
		}

		// Re-register before establishing a new heartbeat stream.
		// This ensures the node exits in the Hivemind registry even after.
		// Hivemind restarts (which clears the in-memory registry)
		if err := s.register(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Warn("[Golem] re-register failed: %v, retrying in %s",
				err, s.cfg.ReconnectInterval)
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.cfg.ReconnectInterval):
			}
			continue
		}

		if err := s.runHeartbeatStream(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Warn("[Golem] heartbeat stream error: %v, reconnecting in %s",
				err, s.cfg.ReconnectInterval)
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.cfg.ReconnectInterval):
			}
		}
	}
}

// runHeartbeatStream opens a single bidirectional heartbeat stream and
// sends periodic heartbeats until the stream fails or context is cancelled.
// It also receives task dispatch and cancel commands from Hivemind.
func (s *Service) runHeartbeatStream(ctx context.Context) error {
	stream, err := s.client.Heartbeat(ctx)
	if err != nil {
		return err
	}

	// Goroutine to receive heartbeat responses (including task dispatches).
	errCh := make(chan error, 1)
	go func() {
		for {
			resp, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					errCh <- nil
				} else {
					errCh <- err
				}
				return
			}
			s.handleHeartbeatResponse(ctx, resp)
		}
	}()

	// Send heartbeat ticker.
	ticker := time.NewTicker(s.cfg.HeartbeatInterval)
	defer ticker.Stop()

	// Send first heartbeat immediately.
	if err := s.sendHeartbeat(stream); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			stream.CloseSend()
			return ctx.Err()
		case err := <-errCh:
			return err
		case <-ticker.C:
			if err := s.sendHeartbeat(stream); err != nil {
				return err
			}
		}
	}
}

// sendHeartbeat sends a single heartbeat message.
func (s *Service) sendHeartbeat(stream pb.GolemNodeService_HeartbeatClient) error {
	loadInfo := s.collectLoadInfo()

	req := &pb.HeartbeatRequest{
		NodeId:    s.nodeID,
		LoadInfo:  loadInfo,
		Timestamp: timestamppb.Now(),
	}

	if err := stream.Send(req); err != nil {
		return err
	}

	logger.Debug("[Golem] heartbeat sent (active_tasks=%d)", loadInfo.ActiveTasks)
	return nil
}

// handleHeartbeatResponse processes control actions from Hivemind heartbeat responses.
// This now handles task dispatch and cancel in addition to drain/shutdown.
func (s *Service) handleHeartbeatResponse(ctx context.Context, resp *pb.HeartbeatResponse) {
	if !resp.Acknowledged {
		logger.Warn("[Golem] heartbeat not acknowledged by Hivemind")
		return
	}

	switch resp.Action {
	case pb.HeartbeatAction_HEARTBEAT_ACTION_NONE:
		// No action required.

	case pb.HeartbeatAction_HEARTBEAT_ACTION_DRAIN:
		logger.Info("[Golem] received DRAIN action from Hivemind")
		s.SetStatus(pb.NodeStatus_NODE_STATUS_DRAINING)
		// TODO: stop accepting new tasks, wait for running tasks to complete

	case pb.HeartbeatAction_HEARTBEAT_ACTION_SHUTDOWN:
		logger.Info("[Golem] received SHUTDOWN action from Hivemind")
		// TODO: initiate graceful shutdown

	case pb.HeartbeatAction_HEARTBEAT_ACTION_DISPATCH_TASK:
		if resp.DispatchTask == nil {
			logger.Warn("[Golem] received DISPATCH_TASK action but task is nil")
			return
		}
		logger.Info("[Golem] received task dispatch via heartbeat: id=%s skill=%s",
			resp.DispatchTask.Id, resp.DispatchTask.SkillName)

		// Delegate to the task handler for async execution.
		if s.taskHandler != nil {
			s.taskHandler.HandleTask(ctx, resp.DispatchTask)
		} else {
			logger.Warn("[Golem] no task handler registered, ignoring task %s", resp.DispatchTask.Id)
		}

	case pb.HeartbeatAction_HEARTBEAT_ACTION_CANCEL_TASK:
		if resp.CancelTaskId == "" {
			logger.Warn("[Golem] received CANCEL_TASK action but task_id is empty")
			return
		}
		logger.Info("[Golem] received task cancel via heartbeat: id=%s reason=%s",
			resp.CancelTaskId, resp.CancelReason)

		if s.taskHandler != nil {
			s.taskHandler.CancelTask(resp.CancelTaskId, resp.CancelReason)
		}

	default:
		logger.Warn("[Golem] unknown heartbeat action: %v", resp.Action)
	}
}
