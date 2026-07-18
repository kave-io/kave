package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	connect "connectrpc.com/connect"
	corev2 "github.com/kave-io/kave/core/v2"
	v2authctx "github.com/kave-io/kave/server/internal/v2/authctx"
	"github.com/kave-io/kave/server/internal/v2/httpapi"
	v2postgres "github.com/kave-io/kave/server/internal/v2/postgres"
	"google.golang.org/protobuf/types/known/emptypb"
)

const testRawServiceKey = "kv2_A1b2C3d4E5f6G7h8I9j0K1l2.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

type fakeAuthenticator struct {
	identity httpapi.Identity
	err      error
	raw      string
	calls    atomic.Int32
}

type fakeDatabaseAuthenticator struct {
	identity v2postgres.ServiceKeyIdentity
	err      error
}

func (f fakeDatabaseAuthenticator) AuthenticateRaw(context.Context, string) (v2postgres.ServiceKeyIdentity, error) {
	return f.identity, f.err
}

func (f *fakeAuthenticator) AuthenticateRaw(_ context.Context, raw string) (httpapi.Identity, error) {
	f.calls.Add(1)
	f.raw = raw
	return f.identity, f.err
}

func TestAuthMiddlewareRejectsBeforeHandler(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		headers []string
	}{
		{name: "missing"},
		{name: "basic", headers: []string{"Basic nope"}},
		{name: "empty bearer", headers: []string{"Bearer "}},
		{name: "duplicate", headers: []string{"Bearer " + testRawServiceKey, "Bearer " + testRawServiceKey}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			authenticator := &fakeAuthenticator{}
			client, handlerCalls := newAuthClient(t, authenticator)
			req := connect.NewRequest(&emptypb.Empty{})
			for _, header := range tc.headers {
				req.Header().Add("Authorization", header)
			}
			_, err := client.CallUnary(context.Background(), req)
			if connect.CodeOf(err) != connect.CodeUnauthenticated {
				t.Fatalf("code = %v, want unauthenticated (err=%v)", connect.CodeOf(err), err)
			}
			if handlerCalls.Load() != 0 || authenticator.calls.Load() != 0 {
				t.Fatalf("handler/authenticator calls = %d/%d, want 0/0", handlerCalls.Load(), authenticator.calls.Load())
			}
		})
	}
}

func TestAuthMiddlewareInjectsDefaultDenyCaller(t *testing.T) {
	t.Parallel()

	authenticator := &fakeAuthenticator{identity: httpapi.Identity{
		AccountID: "account/acme", NamespaceID: "namespace/prod", ServiceKeyID: "key/worker",
		Operations: []corev2.Operation{corev2.OperationConsume}, AllowedAgentIDs: []corev2.Ref{"agent/assistant"}, CanAssertScope: true,
	}}
	var got corev2.Caller
	client, _ := newAuthClientWithObserver(t, authenticator, func(ctx context.Context) {
		got, _ = v2authctx.CallerFrom(ctx)
	})
	req := connect.NewRequest(&emptypb.Empty{})
	req.Header().Set("Authorization", "Bearer "+testRawServiceKey)
	if _, err := client.CallUnary(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if authenticator.raw != testRawServiceKey {
		t.Fatalf("raw key = %q", authenticator.raw)
	}
	if got.AccountID != "account/acme" || got.NamespaceID != "namespace/prod" || got.ServiceKeyID != "key/worker" {
		t.Fatalf("caller = %+v", got)
	}
	if len(got.Operations) != 1 || got.Operations[0] != corev2.OperationConsume || len(got.AllowedAgentIDs) != 1 {
		t.Fatalf("capabilities were not narrowed: %+v", got)
	}
}

func TestAuthMiddlewareMasksInvalidAndSurfacesUnavailable(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
		code connect.Code
	}{
		{name: "invalid", err: v2postgres.ErrInvalidServiceKey, code: connect.CodeUnauthenticated},
		{name: "database unavailable", err: errors.New("database leaked detail"), code: connect.CodeUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, calls := newAuthClient(t, &fakeAuthenticator{err: tc.err})
			req := connect.NewRequest(&emptypb.Empty{})
			req.Header().Set("Authorization", "Bearer "+testRawServiceKey)
			_, err := client.CallUnary(context.Background(), req)
			if connect.CodeOf(err) != tc.code {
				t.Fatalf("code = %v, want %v (err=%v)", connect.CodeOf(err), tc.code, err)
			}
			if calls.Load() != 0 {
				t.Fatalf("handler calls = %d", calls.Load())
			}
			if err != nil && tc.code == connect.CodeUnavailable && errors.Is(err, tc.err) {
				t.Fatal("internal database error leaked through transport")
			}
		})
	}
}

func TestDatabaseAuthenticatorNarrowsPersistedCapabilities(t *testing.T) {
	t.Parallel()
	auth := httpapi.NewDatabaseAuthenticator(fakeDatabaseAuthenticator{identity: v2postgres.ServiceKeyIdentity{
		AccountID:       "account/acme",
		NamespaceID:     "namespace/prod",
		ServiceKeyID:    "key/worker",
		Capabilities:    []string{"consume", "unknown-capability"},
		AllowedAgentIDs: []string{"agent/assistant"},
		CanAssertScope:  true,
	}})
	identity, err := auth.AuthenticateRaw(context.Background(), testRawServiceKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(identity.Operations) != 1 || identity.Operations[0] != corev2.OperationConsume {
		t.Fatalf("operations = %v", identity.Operations)
	}
	if len(identity.AllowedAgentIDs) != 1 || identity.AllowedAgentIDs[0] != "agent/assistant" {
		t.Fatalf("allowed agents = %v", identity.AllowedAgentIDs)
	}
}

func newAuthClient(t *testing.T, authenticator httpapi.Authenticator) (*connect.Client[emptypb.Empty, emptypb.Empty], *atomic.Int32) {
	t.Helper()
	return newAuthClientWithObserver(t, authenticator, func(context.Context) {})
}

func newAuthClientWithObserver(t *testing.T, authenticator httpapi.Authenticator, observe func(context.Context)) (*connect.Client[emptypb.Empty, emptypb.Empty], *atomic.Int32) {
	t.Helper()
	const procedure = "/kave.kernel.v2.TestAuth/Call"
	var handlerCalls atomic.Int32
	handler := connect.NewUnaryHandlerSimple(procedure, func(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
		handlerCalls.Add(1)
		observe(ctx)
		return &emptypb.Empty{}, nil
	})
	mux := http.NewServeMux()
	mux.Handle(procedure, httpapi.NewAuthMiddleware(authenticator).WrapConnect(handler))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return connect.NewClient[emptypb.Empty, emptypb.Empty](server.Client(), server.URL+procedure), &handlerCalls
}
