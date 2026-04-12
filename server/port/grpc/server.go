package grpcport

import (
	"fmt"
	"net"

	"github.com/kave-io/kave/core/store"
	"google.golang.org/grpc"
)

// Server exposes the control-plane API over gRPC.
// Service handlers are registered here once proto generation is complete.
type Server struct {
	app  store.AppStore
	grpc *grpc.Server
}

// New constructs a gRPC server.
func New(app store.AppStore) *Server {
	srv := &Server{
		app:  app,
		grpc: grpc.NewServer(),
	}
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
