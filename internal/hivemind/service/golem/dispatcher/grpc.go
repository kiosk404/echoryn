package dispatcher

import (
	"context"
	"fmt"

	"github.com/kiosk404/echoryn/pkg/logger"
	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
)

// StreamDispatcher implements Dispatcher using bidirectional heartbeat streams.
// Instead of establishing reverse gRPC connections to Golem nodes,
// it sends tasks through the existing heartbeat stream that Golem initiated.
type StreamDispatcher struct {
	streamMgr StreamManager
}

var _ Dispatcher = (*StreamDispatcher)(nil)

// NewStreamDispatcher creates a new stream-based dispatcher
func NewStreamDispatcher(streamMgr StreamManager) *StreamDispatcher {
	return &StreamDispatcher{
		streamMgr: streamMgr,
	}
}

// SetStreamManager binds the StreamManager to this dispatcher.
// This is called after the NodeServiceHandler is created.
func (d *StreamDispatcher) SetStreamManager(mgr StreamManager) {
	d.streamMgr = mgr
}

func (d *StreamDispatcher) Dispatch(ctx context.Context, nodeID string, task *pb.Task) (*pb.DispatchTaskResponse, error) {
	resp := &pb.HeartbeatResponse{
		Acknowledged: true,
		Action:       pb.HeartbeatAction_HEARTBEAT_ACTION_DISPATCH_TASK,
		DispatchTask: task,
	}

	if err := d.streamMgr.SendToNode(nodeID, resp); err != nil {
		return nil, fmt.Errorf("failed to dispatch task %s to node %s via stream: %w", task.Id, nodeID, err)
	}

	logger.Info("[StreamDispatcher] dispatched task %s to %v via heartbeat stream", task.Id, nodeID)

	// Wait for the dispatch acknowledgement from Golem via the heartbeat request.
	result, err := d.streamMgr.WaitForResult(ctx, nodeID, task.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to get dispatch result for task %s from node %s: %w", task.Id, nodeID, err)
	}
	return result, nil
}

func (d *StreamDispatcher) CancelTask(ctx context.Context, nodeID string, taskID string, reason string) error {
	resp := &pb.HeartbeatResponse{
		Acknowledged: true,
		Action:       pb.HeartbeatAction_HEARTBEAT_ACTION_CANCEL_TASK,
		CancelTaskId: taskID,
		CancelReason: reason,
	}

	if err := d.streamMgr.SendToNode(nodeID, resp); err != nil {
		return fmt.Errorf("failed to cancel task %s on node %s via heartbeat stream", taskID, nodeID)
	}
	return nil
}

func (d *StreamDispatcher) Start(ctx context.Context) error {
	logger.Info("[StreamDispatcher] started (stream-based, no reverse connections)")
	return nil
}

func (d *StreamDispatcher) Stop(ctx context.Context) error {
	logger.Info("[StreamDispatcher] stopped")
	return nil
}
