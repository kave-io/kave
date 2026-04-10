// Package handlers provides the HTTP handlers for the Kave control plane.
// Routes are registered under /api/v1 and served by the stdlib net/http mux.
package handlers

import (
	"net/http"

	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/ops/cost"
	httprouter "github.com/kave-io/kave/server/port/http/router"
)

// API holds the dependencies shared across all handlers.
type API struct {
	app    store.AppStore
	span   store.SpanStore
	prices *cost.Service
	encKey []byte // optional AES-256 key for credential encryption; nil = plaintext
}

// New creates a new API with the given stores.
// encKey may be nil (dev mode — credentials stored as plaintext).
func New(app store.AppStore, span store.SpanStore, prices *cost.Service, encKey []byte) *API {
	return &API{app: app, span: span, prices: prices, encKey: encKey}
}

// RegisterRoutes registers all /api/v1 routes on mux.
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	httprouter.Register(mux, []httprouter.Route{
		{Pattern: "POST /api/v1/workspaces", Handler: a.createWorkspace},
		{Pattern: "GET /api/v1/workspaces/{id}", Handler: a.getWorkspace},
		{Pattern: "POST /api/v1/agents", Handler: a.createAgent},
		{Pattern: "GET /api/v1/agents", Handler: a.listAgents},
		{Pattern: "GET /api/v1/agents/{id}", Handler: a.getAgent},
		{Pattern: "PATCH /api/v1/agents/{id}", Handler: a.updateAgent},
		{Pattern: "POST /api/v1/policies", Handler: a.createPolicy},
		{Pattern: "GET /api/v1/policies/{id}", Handler: a.getPolicy},
		{Pattern: "GET /api/v1/runs", Handler: a.listRuns},
		{Pattern: "GET /api/v1/runs/{id}", Handler: a.getRun},
		{Pattern: "GET /api/v1/runs/{id}/spans", Handler: a.getRunSpans},
		{Pattern: "GET /api/v1/spans", Handler: a.listSpans},
		{Pattern: "GET /api/v1/spans/stream", Handler: a.watchSpans},
		{Pattern: "POST /api/v1/credentials", Handler: a.storeCredential},
		{Pattern: "GET /api/v1/cost/summary", Handler: a.costSummary},
		{Pattern: "GET /api/v1/settings/pricing", Handler: a.getPricing},
		{Pattern: "PUT /api/v1/settings/pricing", Handler: a.putPricing},
	})
}
