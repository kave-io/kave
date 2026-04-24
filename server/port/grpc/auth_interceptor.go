package grpcport

import (
	"context"

	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/authctx"
	serverauth "github.com/kave-io/kave/server/ops/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func NewAuthUnaryInterceptor(app store.AppStore, tokens *serverauth.TokenManager, allowAnonymous, allowLegacy bool) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		id, err := identityFromMetadata(ctx, app, tokens, allowLegacy)
		if err != nil {
			id = authctx.Identity{Kind: authctx.KindInvalid, Err: err.Error()}
		}
		if id.IsAnonymous() && !allowAnonymous {
			id.Kind = authctx.KindInvalid
			id.Err = serverauth.ErrUnauthenticated.Error()
		}
		ctx = authctx.With(ctx, id)
		return handler(ctx, req)
	}
}

func NewAuthStreamInterceptor(app store.AppStore, tokens *serverauth.TokenManager, allowAnonymous, allowLegacy bool) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		id, err := identityFromMetadata(ctx, app, tokens, allowLegacy)
		if err != nil {
			id = authctx.Identity{Kind: authctx.KindInvalid, Err: err.Error()}
		}
		if id.IsAnonymous() && !allowAnonymous {
			id.Kind = authctx.KindInvalid
			id.Err = serverauth.ErrUnauthenticated.Error()
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
	if vals := md.Get("authorization"); len(vals) > 0 {
		authHeader = vals[0]
	}
	return serverauth.ParseIdentity(ctx, authHeader, app, tokens, allowLegacy)
}

type serverStreamWithContext struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *serverStreamWithContext) Context() context.Context { return s.ctx }
