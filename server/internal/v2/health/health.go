// Package health owns Kave's production liveness and dependency-aware
// readiness endpoints.
package health

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/kave-io/kave/server/internal/v2/observability"
)

type Check func(context.Context) error

type Dependency struct {
	// Name is operator-controlled code, never request or tenant input.
	Name  string
	Check Check
}

type Handler struct {
	timeout      time.Duration
	dependencies []Dependency
	metrics      *observability.Metrics
	logger       *slog.Logger
	now          func() time.Time
}

func New(timeout time.Duration, dependencies []Dependency, metrics *observability.Metrics, logger *slog.Logger) *Handler {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		timeout: timeout, dependencies: append([]Dependency(nil), dependencies...),
		metrics: metrics, logger: logger, now: time.Now,
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /livez", h.Liveness)
	mux.HandleFunc("HEAD /livez", h.Liveness)
	mux.HandleFunc("GET /readyz", h.Readiness)
	mux.HandleFunc("HEAD /readyz", h.Readiness)
}

func (h *Handler) Liveness(w http.ResponseWriter, r *http.Request) {
	writeStatus(w, r, http.StatusOK, "alive", h.now().UTC())
}

func (h *Handler) Readiness(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	ready := true
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	for _, dependency := range h.dependencies {
		if dependency.Check == nil {
			ready = false
			h.logger.Warn("runtime readiness check failed", "dependency", dependency.Name, "reason", "check_unavailable")
			break
		}
		if err := dependency.Check(ctx); err != nil {
			ready = false
			// Dependency names are fixed at assembly; the concrete database error
			// is deliberately omitted because readiness endpoints are public.
			h.logger.Warn("runtime readiness check failed", "dependency", dependency.Name, "reason", "dependency_unavailable")
			break
		}
	}
	h.metrics.ObserveReadiness(ready, time.Since(started))
	if !ready {
		writeStatus(w, r, http.StatusServiceUnavailable, "not_ready", h.now().UTC())
		return
	}
	writeStatus(w, r, http.StatusOK, "ready", h.now().UTC())
}

func writeStatus(w http.ResponseWriter, r *http.Request, status int, state string, checkedAt time.Time) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(struct {
		Status      string `json:"status"`
		CheckedAtMS int64  `json:"checked_at_ms"`
	}{Status: state, CheckedAtMS: checkedAt.UnixMilli()})
}
