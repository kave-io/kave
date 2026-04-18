package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/kave-io/kave/core/bus"
	"github.com/kave-io/kave/core/store"
)

// Handler is the API HTTP handler registry.
type Handler struct {
	app store.AppStore
	spans store.SpanStore
	bus *bus.Bus
}

// New creates a new API handler.
func New(app store.AppStore, spans store.SpanStore, b *bus.Bus) *Handler {
	return &Handler{
		app:   app,
		spans: spans,
		bus:   b,
	}
}

// RegisterRoutes registers all API routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/agents", h.listAgents)
	mux.HandleFunc("GET /api/v1/agents/{id}", h.getAgent)
	mux.HandleFunc("POST /api/v1/agents", h.createAgent)
	mux.HandleFunc("PATCH /api/v1/agents/{id}", h.updateAgent)

	mux.HandleFunc("GET /api/v1/policies/{id}", h.getPolicy)
	mux.HandleFunc("POST /api/v1/policies", h.createPolicy)
	mux.HandleFunc("GET /api/v1/policies", h.listPolicies)

	mux.HandleFunc("GET /api/v1/runs", h.listRuns)
	mux.HandleFunc("GET /api/v1/runs/{id}", h.getRun)
	mux.HandleFunc("GET /api/v1/runs/{id}/spans", h.getRunSpans)

	mux.HandleFunc("GET /api/v1/spans", h.listSpans)
	mux.HandleFunc("GET /api/v1/spans/stream", h.streamSpans)

	mux.HandleFunc("GET /api/v1/cost/summary", h.getCostSummary)

	mux.HandleFunc("GET /api/v1/settings/pricing", h.getPriceBook)
	mux.HandleFunc("PUT /api/v1/settings/pricing", h.updatePriceBook)
}

// responseJSON writes a JSON response.
func responseJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// errorResponse is an error response.
type errorResponse struct {
	Error string `json:"error"`
}

// errorJSON writes an error response.
func errorJSON(w http.ResponseWriter, status int, msg string) {
	responseJSON(w, status, errorResponse{Error: msg})
}

// pageQuery parses limit and cursor from query params.
func pageQuery(r *http.Request) store.Page {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	cursor := r.URL.Query().Get("cursor")
	return store.Page{Limit: limit, Cursor: cursor}
}

// getPathParam gets a path parameter from the request.
func getPathParam(r *http.Request, name string) string {
	return r.PathValue(name)
}

// getQueryParam gets a query parameter from the request.
func getQueryParam(r *http.Request, name string) string {
	return r.URL.Query().Get(name)
}

// getQueryParamBool gets a boolean query parameter.
func getQueryParamBool(r *http.Request, name string) *bool {
	v := r.URL.Query().Get(name)
	if v == "" {
		return nil
	}
	b := strings.ToLower(v) == "true"
	return &b
}
