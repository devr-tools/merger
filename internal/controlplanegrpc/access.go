package controlplanegrpc

import (
	"context"
	"errors"

	"github.com/devr-tools/merger/internal/access"
	mergerv1 "github.com/devr-tools/merger/proto/merger/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AccessUnaryServerInterceptor authenticates gRPC metadata and enforces the
// role assigned to each control-plane method.
func AccessUnaryServerInterceptor(authenticator access.Authenticator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		role, ok := roleForMethod(info.FullMethod)
		if !ok {
			return nil, status.Error(codes.PermissionDenied, "method has no access policy")
		}

		authorization := authorizationFromMetadata(ctx)
		principal, err := authenticator.Authenticate(authorization)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "valid bearer credentials are required")
		}
		if err := access.Authorize(principal, role); err != nil {
			if errors.Is(err, access.ErrUnauthenticated) {
				return nil, status.Error(codes.Unauthenticated, "valid bearer credentials are required")
			}
			return nil, status.Error(codes.PermissionDenied, "principal does not have the required role")
		}

		return handler(access.WithPrincipal(ctx, principal), request)
	}
}

func roleForMethod(fullMethod string) (access.Role, bool) {
	switch fullMethod {
	case mergerv1.ChangeControlService_GetChangePacket_FullMethodName,
		mergerv1.ChangeControlService_ListChangePackets_FullMethodName,
		mergerv1.ChangeControlService_ListEvidenceAuditEntries_FullMethodName:
		return access.RoleReader, true
	case mergerv1.ChangeControlService_UpdateEvidenceExecution_FullMethodName:
		return access.RoleEvidenceWriter, true
	default:
		return "", false
	}
}

func authorizationFromMetadata(ctx context.Context) string {
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(values) != 1 {
		return ""
	}
	return values[0]
}
