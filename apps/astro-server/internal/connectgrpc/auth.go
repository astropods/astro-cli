package connectgrpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/postman/astro/apps/astro-server/internal/auth"
)

type ctxKey string

const (
	ctxUserID ctxKey = "user_id"
	ctxOrgID  ctxKey = "org_id"
)

// UserIDFromContext extracts the authenticated user ID from the context.
func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxUserID).(string)
	return v
}

// OrgIDFromContext extracts the authenticated organization ID from the context.
func OrgIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxOrgID).(string)
	return v
}

// JWTStreamInterceptor returns a gRPC stream interceptor that validates
// Bearer JWT tokens from the "authorization" metadata key.
func JWTStreamInterceptor(validator *auth.JWTValidator) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return status.Error(codes.Unauthenticated, "missing metadata")
		}

		vals := md.Get("authorization")
		if len(vals) == 0 {
			return status.Error(codes.Unauthenticated, "missing authorization header")
		}

		token := vals[0]
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}

		claims, err := validator.ValidateToken(ctx, token)
		if err != nil {
			return status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
		}

		// Inject user identity into context
		ctx = context.WithValue(ctx, ctxUserID, claims.Subject)
		ctx = context.WithValue(ctx, ctxOrgID, claims.OrganizationID)

		wrapped := &wrappedStream{ServerStream: ss, ctx: ctx}
		return handler(srv, wrapped)
	}
}

// wrappedStream overrides Context() on the server stream to carry auth values.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }
