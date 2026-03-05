package grpc

import (
	"context"

	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/registry"
	"github.com/kiosk404/echoryn/pkg/logger"
	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// AdminServiceHandler implements pb.HivemindAdminServiceServer.
// It provides administrative operations on Golem nodes.
type AdminServiceHandler struct {
	pb.UnimplementedHivemindAdminServiceServer
	registry registry.Registry
}

// NewAdminServiceHandler creates a new AdminServiceHandler.
func NewAdminServiceHandler(reg registry.Registry) *AdminServiceHandler {
	return &AdminServiceHandler{registry: reg}
}

// ListNodes returns a list of registered Golem nodes.
func (h *AdminServiceHandler) ListNodes(ctx context.Context, req *pb.ListNodesRequest) (*pb.ListNodesResponse, error) {
	filter := &registry.NodeFilter{
		StatusFilter: req.StatusFilter,
		PageSize:     req.PageSize,
		PageToken:    req.PageToken,
	}

	states, err := h.registry.ListNodes(filter)
	if err != nil {
		return &pb.ListNodesResponse{
			BaseResp: &pb.BaseResp{StatusCode: 1, StatusMessage: err.Error()},
		}, nil
	}

	nodes := make([]*pb.NodeInfo, 0, len(states))
	for _, s := range states {
		nodes = append(nodes, nodeStateToProto(s))
	}

	return &pb.ListNodesResponse{
		Nodes:      nodes,
		TotalCount: int32(len(nodes)),
		BaseResp:   &pb.BaseResp{StatusCode: 0, StatusMessage: "ok"},
	}, nil
}

// GetNode returns details of a single Golem node.
func (h *AdminServiceHandler) GetNode(ctx context.Context, req *pb.GetNodeRequest) (*pb.GetNodeResponse, error) {
	state, err := h.registry.GetNode(req.NodeId)
	if err != nil {
		return &pb.GetNodeResponse{
			BaseResp: &pb.BaseResp{StatusCode: 1, StatusMessage: err.Error()},
		}, nil
	}

	return &pb.GetNodeResponse{
		NodeInfo: nodeStateToProto(state),
		LoadInfo: state.Status.Load,
		BaseResp: &pb.BaseResp{StatusCode: 0, StatusMessage: "ok"},
	}, nil
}

// CordonNode marks a Golem node as unschedulable.
func (h *AdminServiceHandler) CordonNode(ctx context.Context, req *pb.CordonNodeRequest) (*pb.CordonNodeResponse, error) {
	err := h.registry.CordonNode(req.NodeId)
	if err != nil {
		logger.Warn("[AdminService] cordon node %s failed: %v", req.NodeId, err)
		return &pb.CordonNodeResponse{
			Success:  false,
			BaseResp: &pb.BaseResp{StatusCode: 1, StatusMessage: err.Error()},
		}, nil
	}

	return &pb.CordonNodeResponse{
		Success:  true,
		BaseResp: &pb.BaseResp{StatusCode: 0, StatusMessage: "ok"},
	}, nil
}

// UncordonNode marks a Golem node as schedulable.
func (h *AdminServiceHandler) UncordonNode(ctx context.Context, req *pb.UncordonNodeRequest) (*pb.UncordonNodeResponse, error) {
	err := h.registry.UncordonNode(req.NodeId)
	if err != nil {
		logger.Warn("[AdminService] uncordon node %s failed: %v", req.NodeId, err)
		return &pb.UncordonNodeResponse{
			Success:  false,
			BaseResp: &pb.BaseResp{StatusCode: 1, StatusMessage: err.Error()},
		}, nil
	}

	return &pb.UncordonNodeResponse{
		Success:  true,
		BaseResp: &pb.BaseResp{StatusCode: 0, StatusMessage: "ok"},
	}, nil
}

// nodeStateToProto converts internal NodeState to proto NodeInfo.
func nodeStateToProto(s *registry.NodeState) *pb.NodeInfo {
	info := &pb.NodeInfo{
		Id:           s.Spec.NodeID,
		Name:         s.Spec.NodeName,
		Address:      s.Spec.GRPCAddress,
		Status:       s.Status.Phase,
		SystemInfo:   s.Spec.SystemInfo,
		Capabilities: s.Spec.Capabilities,
		Labels:       s.Spec.Labels,
		Version:      s.Spec.Version,
		RegisteredAt: timestamppb.New(s.Status.RegisteredAt),
		LastSeenAt:   timestamppb.New(s.Status.LastHeartbeat),
	}
	return info
}
