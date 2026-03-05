package dispatcher

import (
	"context"

	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
)

// Dispatcher sends task to Golem nodes for execution.
// In the stream-based architecture, tasks are dispatched through
// the bidirectional heartbeat stream rather than reverse gRPC connections.
type Dispatcher interface {
	// Dispatch sends a task to the specified node via its heartbeat stream.
	Dispatch(ctx context.Context, nodeID string, task *pb.Task) (*pb.DispatchTaskResponse, error)

	// CancelTask sends a cancel command to the specified node via its heartbeat stream.
	CancelTask(ctx context.Context, nodeID string, taskID string, reason string) error

	// Start starts the dispatcher.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the dispatcher.
	Stop(ctx context.Context) error
}

// StreamManager manages per-node heartbeat streams for task dispatch.
// It is implemented by the NodeServiceHandler and used by the Dispatcher.
type StreamManager interface {
	// SendToNode sends a HeartbeatResponse to the specified node's heartbeat stream.
	SendToNode(nodeID string, resp *pb.HeartbeatResponse) error

	// WaitForResult waits for a dispatch result from the specified node for the given task.
	WaitForResult(ctx context.Context, nodeID string, taskID string) (*pb.DispatchTaskResponse, error)
}
