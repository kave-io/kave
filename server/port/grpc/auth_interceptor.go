package grpcport

import (
	"context"

	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/authctx"
	serverauth "github.com/kave-io/kave/server/ops/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func NewAuthUnaryInterceptor(app store.AppStore, tokens *serverauth.TokenManager, allowAnonymous, allowLegacy bool) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// These methods must be reachable without a token.
		if isPublicMethod(info.FullMethod) {
			return handler(ctx, req)
		}
		id, err := identityFromMetadata(ctx, app, tokens, allowLegacy)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, serverauth.ErrUnauthenticated.Error())
		}
		if id.IsAnonymous() && !allowAnonymous {
			return nil, status.Error(codes.Unauthenticated, serverauth.ErrUnauthenticated.Error())
		}
		if id.IsInvalid() {
			return nil, status.Error(codes.Unauthenticated, serverauth.ErrUnauthenticated.Error())
		}
		ctx = authctx.With(ctx, id)
		return handler(ctx, req)
	}
}

func NewAuthStreamInterceptor(app store.AppStore, tokens *serverauth.TokenManager, allowAnonymous, allowLegacy bool) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// These methods must be reachable without a token.
		if isPublicMethod(info.FullMethod) {
			return handler(srv, ss)
		}
		ctx := ss.Context()
		id, err := identityFromMetadata(ctx, app, tokens, allowLegacy)
		if err != nil {
			return status.Error(codes.Unauthenticated, serverauth.ErrUnauthenticated.Error())
		}
		if id.IsAnonymous() && !allowAnonymous {
			return status.Error(codes.Unauthenticated, serverauth.ErrUnauthenticated.Error())
		}
		if id.IsInvalid() {
			return status.Error(codes.Unauthenticated, serverauth.ErrUnauthenticated.Error())
		}
		wrapped := &serverStreamWithContext{ServerStream: ss, ctx: authctx.With(ctx, id)}
		return handler(srv, wrapped)
	}
}

func identityFromMetadata(ctx context.Context, app store.AppStore, tokens *serverauth.TokenManager, allowLegacy bool) (authctx.Identity, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if md == nil {
		return authctx.Identity{Kind: authctx.KindAnonymous}, nil
	}
	authHeader := ""
	if vals := md.Get("authorization"); len(vals) > 1 {
		return authctx.Identity{}, serverauth.ErrUnauthenticated
	} else if len(vals) == 1 {
		authHeader = vals[0]
	}
	return serverauth.ParseIdentity(ctx, authHeader, app, tokens, allowLegacy)
}

func isPublicMethod(fullMethod string) bool {
	switch fullMethod {
	case "/kave.control.v1.AuthService/Register",
		"/kave.control.v1.AuthService/Login":
		return true
	}
	return false
}

type serverStreamWithContext struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *serverStreamWithContext) Context() context.Context { return s.ctx }
