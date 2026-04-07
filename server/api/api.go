// Package api provides the REST API handlers for the Kave control plane.
// Routes are registered under /api/v1 and served by the stdlib net/http mux.
package api

import (
	"net/http"

	"github.com/kave-io/kave/core/store"
)

// API holds the dependencies shared across all handlers.
type API struct {
	app    store.AppStore
	span   store.SpanStore
	encKey []byte // optional AES-256 key for credential encryption; nil = plaintext
}

// New creates a new API with the given stores.
// encKey may be nil (dev mode — credentials stored as plaintext).
func New(app store.AppStore, span store.SpanStore, encKey []byte) *API {
	return &API{app: app, span: span, encKey: encKey}
}

// RegisterRoutes registers all /api/v1 routes on mux.
// Uses Go 1.22+ method+path pattern syntax.
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	// Workspaces
	mux.HandleFunc("POST /api/v1/workspaces", a.createWorkspace)
	mux.HandleFunc("GET /api/v1/workspaces/{id}", a.getWorkspace)

	// Agents
	mux.HandleFunc("POST /api/v1/agents", a.createAgent)
	mux.HandleFunc("GET /api/v1/agents", a.listAgents)
	mux.HandleFunc("GET /api/v1/agents/{id}", a.getAgent)
	mux.HandleFunc("PATCH /api/v1/agents/{id}", a.updateAgent)

	// Policies
	mux.HandleFunc("POST /api/v1/policies", a.createPolicy)
	mux.HandleFunc("GET /api/v1/policies/{id}", a.getPolicy)

	// Runs
	mux.HandleFunc("GET /api/v1/runs", a.listRuns)
	mux.HandleFunc("GET /api/v1/runs/{id}", a.getRun)
	mux.HandleFunc("GET /api/v1/runs/{id}/spans", a.getRunSpans)

	// Spans
	mux.HandleFunc("GET /api/v1/spans", a.listSpans)
	mux.HandleFunc("GET /api/v1/spans/stream", a.watchSpans)

	// Credentials
	mux.HandleFunc("POST /api/v1/credentials", a.storeCredential)

	// Cost
	mux.HandleFunc("GET /api/v1/cost/summary", a.costSummary)
}
