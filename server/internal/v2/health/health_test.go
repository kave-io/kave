package health

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kave-io/kave/server/internal/v2/observability"
)

func TestReadinessFailsClosedWithoutLeakingDependencyError(t *testing.T) {
	t.Parallel()
	metrics := observability.New(nil)
	sensitive := "postgres://user:secret@private-db/tenant"
	handler := New(time.Second, []Dependency{{Name: "postgres", Check: func(context.Context) error {
		return errors.New(sensitive)
	}}}, metrics, slog.New(slog.NewTextHandler(io.Discard, nil)))

	response := httptest.NewRecorder()
	handler.Readiness(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"status":"not_ready"`) {
		t.Fatalf("readiness = %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), sensitive) {
		t.Fatal("readiness response leaked dependency error")
	}

	metricsResponse := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(metricsResponse.Body.String(), `kave_v2_readiness_checks_total{result="not_ready"} 1`) ||
		strings.Contains(metricsResponse.Body.String(), sensitive) {
		t.Fatalf("readiness metrics = %s", metricsResponse.Body.String())
	}
}

func TestReadinessHonorsTimeoutAndLivenessStaysIndependent(t *testing.T) {
	t.Parallel()
	handler := New(20*time.Millisecond, []Dependency{{Name: "postgres", Check: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}}}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	started := time.Now()
	ready := httptest.NewRecorder()
	handler.Readiness(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusServiceUnavailable || time.Since(started) > time.Second {
		t.Fatalf("timed readiness = %d after %v", ready.Code, time.Since(started))
	}
	live := httptest.NewRecorder()
	handler.Liveness(live, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if live.Code != http.StatusOK || !strings.Contains(live.Body.String(), `"status":"alive"`) {
		t.Fatalf("liveness = %d %s", live.Code, live.Body.String())
	}
}

func TestReadinessRequiresEveryDependency(t *testing.T) {
	t.Parallel()
	calls := 0
	handler := New(time.Second, []Dependency{
		{Name: "postgres", Check: func(context.Context) error { calls++; return nil }},
		{Name: "role_migrations_keyring", Check: func(context.Context) error { calls++; return nil }},
	}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.Readiness(response, httptest.NewRequest(http.MethodHead, "/readyz", nil))
	if response.Code != http.StatusOK || response.Body.Len() != 0 || calls != 2 {
		t.Fatalf("readiness = %d body=%q calls=%d", response.Code, response.Body.String(), calls)
	}
}
