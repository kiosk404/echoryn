package grpc

import (
	"context"

	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/registry"
	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/tokenmanager"
	"github.com/kiosk404/echoryn/pkg/logger"
	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// AdminServiceHandler implements pb.HivemindAdminServiceServer.
// It provides administrative operations on Golem nodes and Bootstrap Tokens.
type AdminServiceHandler struct {
	pb.UnimplementedHivemindAdminServiceServer
	registry     registry.Registry
	tokenManager tokenmanager.TokenManager
}

// NewAdminServiceHandler creates a new AdminServiceHandler.
func NewAdminServiceHandler(reg registry.Registry, tm tokenmanager.TokenManager) *AdminServiceHandler {
	return &AdminServiceHandler{
		registry:     reg,
		tokenManager: tm,
	}
}

// =========== Node Management ===========

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

// ========== Token Management ==========

// CreateToken creates a new Bootstrap Token.
func (h *AdminServiceHandler) CreateToken(ctx context.Context, req *pb.CreateTokenRequest) (*pb.CreateTokenResponse, error) {
	ttl := req.Ttl.AsDuration()

	fullToken, info, err := h.tokenManager.CreateToken(ctx, ttl, req.MaxUsages, req.Description, req.Labels)
	if err != nil {
		return &pb.CreateTokenResponse{
			BaseResp: &pb.BaseResp{StatusCode: 1, StatusMessage: err.Error()},
		}, nil
	}

	return &pb.CreateTokenResponse{
		Token:     fullToken,
		TokenInfo: bootstrapTokenToProto(info),
		BaseResp:  &pb.BaseResp{StatusCode: 0, StatusMessage: "ok"},
	}, nil
}

// ListTokens lists all Bootstrap Tokens.
func (h *AdminServiceHandler) ListTokens(ctx context.Context, req *pb.ListTokensRequest) (*pb.ListTokensResponse, error) {
	tokens, err := h.tokenManager.ListTokens(ctx)
	if err != nil {
		return &pb.ListTokensResponse{
			BaseResp: &pb.BaseResp{StatusCode: 1, StatusMessage: err.Error()},
		}, nil
	}

	pbTokens := make([]*pb.BootstrapToken, 0, len(tokens))
	for _, t := range tokens {
		pbTokens = append(pbTokens, bootstrapTokenToProto(t))
	}

	return &pb.ListTokensResponse{
		Tokens:     pbTokens,
		TotalCount: int32(len(tokens)),
		BaseResp:   &pb.BaseResp{StatusCode: 0, StatusMessage: "ok"},
	}, nil
}

// DeleteToken deletes a Bootstrap Token.
func (h *AdminServiceHandler) DeleteToken(ctx context.Context, req *pb.DeleteTokenRequest) (*pb.DeleteTokenResponse, error) {
	if err := h.tokenManager.DeleteToken(ctx, req.TokenId); err != nil {
		return &pb.DeleteTokenResponse{
			Success:  false,
			BaseResp: &pb.BaseResp{StatusCode: 1, StatusMessage: err.Error()},
		}, nil
	}

	return &pb.DeleteTokenResponse{
		Success:  true,
		BaseResp: &pb.BaseResp{StatusCode: 0, StatusMessage: "ok"},
	}, nil
}

// ========== Converters ==========

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

// bootstrapTokenToProto
func bootstrapTokenToProto(t *tokenmanager.BootstrapToken) *pb.BootstrapToken {
	return &pb.BootstrapToken{
		Id:          t.ID,
		ExpiresAt:   timestamppb.New(t.ExpiresAt),
		Usages:      t.Usages,
		MaxUsages:   t.MaxUsages,
		Description: t.Description,
		CreatedAt:   timestamppb.New(t.CreatedAt),
		CreatedBy:   t.CreatedBy,
		Labels:      t.Labels,
	}
}
