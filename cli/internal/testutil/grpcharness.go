// Package testutil provides a bufconn-based gRPC test harness for CLI command tests.
package testutil

import (
	"context"
	"net"
	"testing"

	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1 << 20 // 1 MB

// GRPCHarness holds a live in-process gRPC server over a bufconn listener.
// Create one per test with NewGRPCHarness; call Close when done.
type GRPCHarness struct {
	srv    *grpc.Server
	lis    *bufconn.Listener
	conn   *grpc.ClientConn
	AuthMD []string // captured authorization metadata values from requests
}

// NewGRPCHarness spins up the given gRPC servers over bufconn and returns a
// harness whose Transport() can be passed directly to CLI command handlers.
func NewGRPCHarness(t *testing.T, runtimeSvc runtimev1.RuntimeServiceServer, controlSvc controlv1.ControlPlaneServiceServer) *GRPCHarness {
	t.Helper()
	h := &GRPCHarness{
		lis: bufconn.Listen(bufSize),
	}

	h.srv = grpc.NewServer(grpc.UnaryInterceptor(h.captureAuthInterceptor))
	if runtimeSvc != nil {
		runtimev1.RegisterRuntimeServiceServer(h.srv, runtimeSvc)
	}
	if controlSvc != nil {
		controlv1.RegisterControlPlaneServiceServer(h.srv, controlSvc)
	}

	go func() {
		if err := h.srv.Serve(h.lis); err != nil && err != grpc.ErrServerStopped {
			t.Logf("bufconn server error: %v", err)
		}
	}()

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return h.lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("bufconn dial: %v", err)
	}
	h.conn = conn

	t.Cleanup(h.Close)
	return h
}

func (h *GRPCHarness) captureAuthInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		h.AuthMD = append(h.AuthMD, md.Get("authorization")...)
	}
	return handler(ctx, req)
}

// RuntimeSvc returns a RuntimeServiceClient wired to the in-process server.
func (h *GRPCHarness) RuntimeSvc() runtimev1.RuntimeServiceClient {
	return runtimev1.NewRuntimeServiceClient(h.conn)
}

// ControlSvc returns a ControlPlaneServiceClient wired to the in-process server.
func (h *GRPCHarness) ControlSvc() controlv1.ControlPlaneServiceClient {
	return controlv1.NewControlPlaneServiceClient(h.conn)
}

// Context returns a context.Context with a Runtime wired to the harness's
// gRPC connection. Pass this to command handler functions in tests.
func (h *GRPCHarness) Context() context.Context {
	t := runtime.NewTransportFromConn(h.conn)
	rt := &runtime.Runtime{}
	rt.InjectTransport(t)
	return runtime.WithContext(context.Background(), rt)
}

// Close shuts down the server and connection.
func (h *GRPCHarness) Close() {
	h.conn.Close()
	h.srv.GracefulStop()
	h.lis.Close()
}
