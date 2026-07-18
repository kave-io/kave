package connectport_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/kave-io/kave/server/internal/authctx"
	serverauth "github.com/kave-io/kave/server/ops/auth"
	connectport "github.com/kave-io/kave/server/port/connect"
	"google.golang.org/protobuf/types/known/emptypb"
)

const authTestKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestAuthMiddlewareRejectsBeforeHandler(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		authorizations []string
	}{
		{name: "missing"},
		{name: "malformed", authorizations: []string{"Basic nope"}},
		{name: "invalid bearer", authorizations: []string{"Bearer not-a-token"}},
		{name: "duplicate", authorizations: []string{"Bearer one", "Bearer two"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			client := newAuthTestClient(t, false, func(context.Context) {
				calls.Add(1)
			})
			req := connect.NewRequest(&emptypb.Empty{})
			for _, authorization := range tc.authorizations {
				req.Header().Add("Authorization", authorization)
			}

			_, err := client.CallUnary(context.Background(), req)
			if connect.CodeOf(err) != connect.CodeUnauthenticated {
				t.Fatalf("code = %v, want unauthenticated (err=%v)", connect.CodeOf(err), err)
			}
			if calls.Load() != 0 {
				t.Fatalf("handler called %d times", calls.Load())
			}
		})
	}
}

func TestAuthMiddlewareInjectsAuthenticatedIdentity(t *testing.T) {
	t.Parallel()

	tokens, err := serverauth.NewTokenManager(authTestKey, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := tokens.IssueSession("usr_test", "ses_test", "acc_test")
	if err != nil {
		t.Fatal(err)
	}

	var got authctx.Identity
	client := newAuthTestClientWithTokens(t, tokens, func(ctx context.Context) {
		got, _ = authctx.From(ctx)
	})
	req := connect.NewRequest(&emptypb.Empty{})
	req.Header().Set("Authorization", "Bearer "+raw)

	if _, err := client.CallUnary(context.Background(), req); err != nil {
		t.Fatalf("call: %v", err)
	}
	if !got.IsUser() || got.UserID != "usr_test" || got.OrgID != "acc_test" {
		t.Fatalf("unexpected identity: %+v", got)
	}
}

func newAuthTestClient(t *testing.T, allowAnonymous bool, observe func(context.Context)) *connect.Client[emptypb.Empty, emptypb.Empty] {
	t.Helper()
	tokens, err := serverauth.NewTokenManager(authTestKey, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return newAuthTestClientConfigured(t, tokens, allowAnonymous, observe)
}

func newAuthTestClientWithTokens(t *testing.T, tokens *serverauth.TokenManager, observe func(context.Context)) *connect.Client[emptypb.Empty, emptypb.Empty] {
	t.Helper()
	return newAuthTestClientConfigured(t, tokens, false, observe)
}

func newAuthTestClientConfigured(t *testing.T, tokens *serverauth.TokenManager, allowAnonymous bool, observe func(context.Context)) *connect.Client[emptypb.Empty, emptypb.Empty] {
	t.Helper()
	const procedure = "/kave.test.v1.AuthBoundary/CallUnary"
	handler := connect.NewUnaryHandlerSimple(procedure, func(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
		observe(ctx)
		return &emptypb.Empty{}, nil
	})
	authenticate := connectport.NewAuthMiddleware(connectport.AuthConfig{
		Tokens:         tokens,
		AllowAnonymous: allowAnonymous,
	})
	mux := http.NewServeMux()
	mux.Handle(procedure, authenticate(handler))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return connect.NewClient[emptypb.Empty, emptypb.Empty](server.Client(), server.URL+procedure)
}
