package grpc

import (
	"context"
	"strings"

	"github.com/kiosk404/echoryn/internal/hivemind/service/golem/tokenmanager"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AdminAuthUnaryInterceptor returns a gRPC UnaryServerInterceptor that validates
// the Admin Token for HivemindAdminService methods.
// Non-admin methods (e.g. GolemNodeService) pass through without authentication.
func AdminAuthUnaryInterceptor(tm tokenmanager.TokenManager) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Only authenticate HivemindAdminService methods.
		if !isAdminServiceMethod(info.FullMethod) {
			return handler(ctx, req)
		}

		token, err := extractBearerToken(ctx)
		if err != nil {
			return nil, err
		}

		if !tm.ValidateAdminToken(token) {
			return nil, status.Error(codes.PermissionDenied, "invalid admin token")
		}

		return handler(ctx, req)
	}
}

// AdminAuthStreamInterceptor returns a gRPC StreamServerInterceptor that validates
// the Admin Token for HivemindAdminService streaming methods.
func AdminAuthStreamInterceptor(tm tokenmanager.TokenManager) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if !isAdminServiceMethod(info.FullMethod) {
			return handler(srv, ss)
		}

		token, err := extractBearerToken(ss.Context())
		if err != nil {
			return err
		}

		if !tm.ValidateAdminToken(token) {
			return status.Error(codes.PermissionDenied, "invalid admin token")
		}

		return handler(srv, ss)
	}
}

// isAdminServiceMethod checks whether the gRPC method belongs to HivemindAdminService.
func isAdminServiceMethod(fullMethod string) bool {
	return strings.Contains(fullMethod, "HivemindAdminService")
}

// extractBearerToken extracts a Bearer token from the gRPC metadata.
func extractBearerToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing metadata")
	}

	authHeader := md.Get("authorization")
	if len(authHeader) == 0 {
		return "", status.Error(codes.Unauthenticated, "missing authorization header")
	}

	token := strings.TrimPrefix(authHeader[0], "Bearer ")
	if token == "" || token == authHeader[0] {
		return "", status.Error(codes.Unauthenticated, "invalid authorization format, expected 'Bearer <token>'")
	}

	return token, nil
}
