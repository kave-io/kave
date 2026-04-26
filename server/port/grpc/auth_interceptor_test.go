package grpcport_test

import (
	"context"
	"testing"

	"github.com/kave-io/kave/server/internal/authctx"
	serverauth "github.com/kave-io/kave/server/ops/auth"
	grpcport "github.com/kave-io/kave/server/port/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const testKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func newTokens(t *testing.T) *serverauth.TokenManager {
	t.Helper()
	tm, err := serverauth.NewTokenManager(testKeyHex, 0, 0)
	if err != nil {
		t.Fatalf("token manager: %v", err)
	}
	return tm
}

func runUnary(interceptor grpc.UnaryServerInterceptor, md metadata.MD) authctx.Identity {
	ctx := context.Background()
	if md != nil {
		ctx = metadata.NewIncomingContext(ctx, md)
	}
	var got authctx.Identity
	_, _ = interceptor(ctx, struct{}{}, &grpc.UnaryServerInfo{}, func(c context.Context, _ any) (any, error) {
		got, _ = authctx.From(c)
		return nil, nil
	})
	return got
}

func TestUnaryInterceptor_NoMetadata_AllowAnonymous(t *testing.T) {
	intc := grpcport.NewAuthUnaryInterceptor(nil, newTokens(t), true, false)
	id := runUnary(intc, nil)
	if !id.IsAnonymous() {
		t.Fatalf("want anonymous, got %+v", id)
	}
}

func TestUnaryInterceptor_NoMetadata_DenyAnonymous(t *testing.T) {
	intc := grpcport.NewAuthUnaryInterceptor(nil, newTokens(t), false, false)
	id := runUnary(intc, nil)
	if !id.IsInvalid() {
		t.Fatalf("want invalid, got %+v", id)
	}
}

func TestUnaryInterceptor_PASETO_User(t *testing.T) {
	tm := newTokens(t)
	tok, _ := tm.IssueSession("u1", "sess1", "org1")
	intc := grpcport.NewAuthUnaryInterceptor(nil, tm, false, false)
	id := runUnary(intc, metadata.Pairs("authorization", "Bearer "+tok))
	if !id.IsUser() || id.UserID != "u1" {
		t.Fatalf("unexpected: %+v", id)
	}
}

func TestUnaryInterceptor_PASETO_Agent(t *testing.T) {
	tm := newTokens(t)
	tok, _ := tm.IssueAgentToken("agn_1", "prj_1", "env_1", "org1")
	intc := grpcport.NewAuthUnaryInterceptor(nil, tm, false, false)
	id := runUnary(intc, metadata.Pairs("authorization", "Bearer kav_"+tok))
	if !id.IsAgentToken() || id.AgentID != "agn_1" {
		t.Fatalf("unexpected: %+v", id)
	}
}

func TestUnaryInterceptor_BadHeader_Invalid(t *testing.T) {
	intc := grpcport.NewAuthUnaryInterceptor(nil, newTokens(t), true, false)
	id := runUnary(intc, metadata.Pairs("authorization", "Basic xx"))
	if !id.IsInvalid() {
		t.Fatalf("want invalid, got %+v", id)
	}
}

// fakeServerStream lets us drive the stream interceptor without a real grpc transport.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeServerStream) Context() context.Context { return s.ctx }

func TestStreamInterceptor_PASETO_User(t *testing.T) {
	tm := newTokens(t)
	tok, _ := tm.IssueSession("u2", "sess2", "org2")
	intc := grpcport.NewAuthStreamInterceptor(nil, tm, false, false)

	parent := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+tok))
	var got authctx.Identity
	err := intc(nil, &fakeServerStream{ctx: parent}, &grpc.StreamServerInfo{}, func(_ any, ss grpc.ServerStream) error {
		got, _ = authctx.From(ss.Context())
		return nil
	})
	if err != nil {
		t.Fatalf("interceptor err: %v", err)
	}
	if !got.IsUser() || got.UserID != "u2" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestStreamInterceptor_DenyAnonymous(t *testing.T) {
	intc := grpcport.NewAuthStreamInterceptor(nil, newTokens(t), false, false)
	var got authctx.Identity
	_ = intc(nil, &fakeServerStream{ctx: context.Background()}, &grpc.StreamServerInfo{}, func(_ any, ss grpc.ServerStream) error {
		got, _ = authctx.From(ss.Context())
		return nil
	})
	if !got.IsInvalid() {
		t.Fatalf("want invalid, got %+v", got)
	}
}
