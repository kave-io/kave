package http

import (
	nethttp "net/http"

	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/ops/cost"
	"github.com/kave-io/kave/server/port/http/handlers"
)

// Server is the public HTTP port for the control-plane API.
type Server struct {
	handlers *handlers.API
}

// New creates a new HTTP API server.
func New(app store.AppStore, span store.SpanStore, prices *cost.Service, encKey []byte) *Server {
	return &Server{handlers: handlers.New(app, span, prices, encKey)}
}

// RegisterRoutes registers all API routes on the provided mux.
func (s *Server) RegisterRoutes(mux *nethttp.ServeMux) {
	s.handlers.RegisterRoutes(mux)
}
