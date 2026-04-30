package grpcport

import (
	"fmt"
	"net"

	appaudit "github.com/kave-io/kave/server/app/audit"
	appcontrol "github.com/kave-io/kave/server/app/control"
	appruntime "github.com/kave-io/kave/server/app/runtime"
	"github.com/kave-io/kave/server/internal/daemon"
	serverauth "github.com/kave-io/kave/server/ops/auth"
	"google.golang.org/grpc"
)

// Server exposes the control-plane and runtime APIs over gRPC.
type Server struct {
	grpc *grpc.Server
}

// New constructs a gRPC server and registers all service handlers.
func New(control *appcontrol.Server, runtime *appruntime.Server, audit *appaudit.Server, daemonState *daemon.State, tokens *serverauth.TokenManager, unary grpc.UnaryServerInterceptor, stream grpc.StreamServerInterceptor) *Server {
	opts := make([]grpc.ServerOption, 0, 2)
	if unary != nil {
		opts = append(opts, grpc.ChainUnaryInterceptor(unary))
	}
	if stream != nil {
		opts = append(opts, grpc.ChainStreamInterceptor(stream))
	}
	g := grpc.NewServer(opts...)
	control.RegisterWithChildren(g, daemonState, tokens)
	runtime.Register(g)
	if audit != nil {
		audit.Register(g)
	}
	srv := &Server{grpc: g}
	return srv
}

// ListenAndServe starts serving on the provided address.
func (s *Server) ListenAndServe(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("grpc: listen on %s: %w", addr, err)
	}
	return s.grpc.Serve(lis)
}

// GracefulStop stops the server after in-flight RPCs complete.
func (s *Server) GracefulStop() {
	s.grpc.GracefulStop()
}

// GRPC returns the underlying grpc.Server for registering additional services.
func (s *Server) GRPC() *grpc.Server {
	return s.grpc
}
