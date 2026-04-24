package grpcport

import (
	"fmt"
	"net"

	appcontrol "github.com/kave-io/kave/server/app/control"
	appruntime "github.com/kave-io/kave/server/app/runtime"
	"google.golang.org/grpc"
)

// Server exposes the control-plane and runtime APIs over gRPC.
type Server struct {
	grpc *grpc.Server
}

// New constructs a gRPC server and registers all service handlers.
func New(control *appcontrol.Server, runtime *appruntime.Server, unary grpc.UnaryServerInterceptor, stream grpc.StreamServerInterceptor) *Server {
	opts := make([]grpc.ServerOption, 0, 2)
	if unary != nil {
		opts = append(opts, grpc.ChainUnaryInterceptor(unary))
	}
	if stream != nil {
		opts = append(opts, grpc.ChainStreamInterceptor(stream))
	}
	g := grpc.NewServer(opts...)
	control.Register(g)
	runtime.Register(g)
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
