package grpcport

import (
	"fmt"
	"net"

	appcontrol "github.com/kave-io/kave/app/control"
	appruntime "github.com/kave-io/kave/app/runtime"
	"google.golang.org/grpc"
)

// Server exposes the control-plane and runtime APIs over gRPC.
type Server struct {
	grpc *grpc.Server
}

// New constructs a gRPC server and registers all service handlers.
func New(control *appcontrol.Server, runtime *appruntime.Server) *Server {
	g := grpc.NewServer()
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
