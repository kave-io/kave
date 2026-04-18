package grpcport

import (
	"fmt"
	"net"

	appcontrol "github.com/kave-io/kave/app/control"
	appruntime "github.com/kave-io/kave/app/runtime"
	"github.com/kave-io/kave/core/bus"
	"github.com/kave-io/kave/core/store"
	"google.golang.org/grpc"
)

// Server exposes the control-plane and runtime APIs over gRPC.
type Server struct {
	app  store.AppStore
	grpc *grpc.Server
}

// New constructs a gRPC server and registers all service handlers.
func New(app store.AppStore, spans store.SpanStore, b *bus.Bus) *Server {
	g := grpc.NewServer()
	appcontrol.New(app).Register(g)
	appruntime.New(app, spans, b).Register(g)
	srv := &Server{app: app, grpc: g}
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
