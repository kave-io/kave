// Package observability provides the deliberately bounded production metrics
// surface for the V2 kernel. Metric labels are selected from fixed enums in
// this package; tenant references, service-key IDs, invocation IDs, provider
// request IDs, models, and routes are never accepted as labels.
package observability

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	corev2 "github.com/kave-io/kave/core/v2"
	"github.com/kave-io/kave/server/internal/v2/httpapi"
	v2postgres "github.com/kave-io/kave/server/internal/v2/postgres"
	"github.com/kave-io/kave/server/internal/v2/provider"
)

var durationBounds = [...]time.Duration{
	5 * time.Millisecond,
	10 * time.Millisecond,
	25 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
	250 * time.Millisecond,
	500 * time.Millisecond,
	time.Second,
	2500 * time.Millisecond,
	5 * time.Second,
	10 * time.Second,
	30 * time.Second,
}

type counterKey struct {
	name   string
	labels string
}

type histogram struct {
	count   uint64
	sum     float64
	buckets [len(durationBounds) + 1]uint64
}

// Metrics is an in-process Prometheus collector with a finite label space.
// A mutex is intentional here: updates are short and the number of series is
// tiny, while a single coherent snapshot makes scrapes deterministic.
type Metrics struct {
	mu         sync.Mutex
	counters   map[counterKey]uint64
	histograms map[counterKey]histogram
	poolStats  func() PoolStats
}

// PoolStats is the safe aggregate subset of pgx pool statistics exposed to
// operators. It contains no connection strings or database identities.
type PoolStats struct {
	Acquired int32
	Idle     int32
	Total    int32
	Max      int32
}

func New(poolStats func() PoolStats) *Metrics {
	return &Metrics{
		counters:   make(map[counterKey]uint64),
		histograms: make(map[counterKey]histogram),
		poolStats:  poolStats,
	}
}

func (m *Metrics) increment(name, labels string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.counters[counterKey{name: name, labels: labels}]++
	m.mu.Unlock()
}

func (m *Metrics) observe(name, labels string, elapsed time.Duration) {
	if m == nil {
		return
	}
	if elapsed < 0 {
		elapsed = 0
	}
	m.mu.Lock()
	key := counterKey{name: name, labels: labels}
	h := m.histograms[key]
	h.count++
	h.sum += elapsed.Seconds()
	placed := false
	for i, bound := range durationBounds {
		if elapsed <= bound {
			h.buckets[i]++
			placed = true
		}
	}
	if !placed || elapsed > durationBounds[len(durationBounds)-1] {
		h.buckets[len(durationBounds)]++
	}
	m.histograms[key] = h
	m.mu.Unlock()
}

// Handler renders only aggregate, fixed-label metrics. The endpoint is safe
// to expose to a production Prometheus scraper without putting tenant data in
// its time-series database.
func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		m.writePrometheus(w)
	})
}

func (m *Metrics) writePrometheus(w io.Writer) {
	if m == nil {
		return
	}
	m.mu.Lock()
	counters := make(map[counterKey]uint64, len(m.counters))
	for key, value := range m.counters {
		counters[key] = value
	}
	histograms := make(map[counterKey]histogram, len(m.histograms))
	for key, value := range m.histograms {
		histograms[key] = value
	}
	m.mu.Unlock()

	keys := make([]counterKey, 0, len(counters))
	for key := range counters {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].name == keys[j].name {
			return keys[i].labels < keys[j].labels
		}
		return keys[i].name < keys[j].name
	})
	for _, key := range keys {
		fmt.Fprintf(w, "%s%s %d\n", key.name, prometheusLabels(key.labels), counters[key])
	}

	histogramKeys := make([]counterKey, 0, len(histograms))
	for key := range histograms {
		histogramKeys = append(histogramKeys, key)
	}
	sort.Slice(histogramKeys, func(i, j int) bool {
		if histogramKeys[i].name == histogramKeys[j].name {
			return histogramKeys[i].labels < histogramKeys[j].labels
		}
		return histogramKeys[i].name < histogramKeys[j].name
	})
	for _, key := range histogramKeys {
		h := histograms[key]
		for i, bound := range durationBounds {
			labels := appendLabel(key.labels, "le", fmt.Sprintf("%g", bound.Seconds()))
			fmt.Fprintf(w, "%s_bucket%s %d\n", key.name, prometheusLabels(labels), h.buckets[i])
		}
		labels := appendLabel(key.labels, "le", "+Inf")
		fmt.Fprintf(w, "%s_bucket%s %d\n", key.name, prometheusLabels(labels), h.count)
		fmt.Fprintf(w, "%s_sum%s %g\n", key.name, prometheusLabels(key.labels), h.sum)
		fmt.Fprintf(w, "%s_count%s %d\n", key.name, prometheusLabels(key.labels), h.count)
	}

	if m.poolStats != nil {
		stats := m.poolStats()
		fmt.Fprintf(w, "kave_v2_postgres_connections{state=\"acquired\"} %d\n", stats.Acquired)
		fmt.Fprintf(w, "kave_v2_postgres_connections{state=\"idle\"} %d\n", stats.Idle)
		fmt.Fprintf(w, "kave_v2_postgres_connections{state=\"total\"} %d\n", stats.Total)
		fmt.Fprintf(w, "kave_v2_postgres_connections{state=\"max\"} %d\n", stats.Max)
	}
}

func prometheusLabels(labels string) string {
	if labels == "" {
		return ""
	}
	return "{" + labels + "}"
}

func appendLabel(labels, name, value string) string {
	entry := name + "=\"" + value + "\""
	if labels == "" {
		return entry
	}
	return labels + "," + entry
}

// ObserveReadiness records only ready/not-ready. Individual dependency
// failures belong in structured logs, not in an unbounded error label.
func (m *Metrics) ObserveReadiness(ready bool, elapsed time.Duration) {
	result := "ready"
	if !ready {
		result = "not_ready"
	}
	labels := "result=\"" + result + "\""
	m.increment("kave_v2_readiness_checks_total", labels)
	m.observe("kave_v2_readiness_check_duration_seconds", labels, elapsed)
}

// ObserveServiceKeyUsageUpdate reports the asynchronous, sampled last-used
// telemetry path. Authentication never depends on this update succeeding.
func (m *Metrics) ObserveServiceKeyUsageUpdate(err error) {
	result := "success"
	if err != nil {
		result = "error"
	}
	m.increment("kave_v2_service_key_usage_updates_total", "result=\""+result+"\"")
}

// HTTPMiddleware tracks only a fixed surface, method, and status class.
func (m *Metrics) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		labels := fmt.Sprintf("surface=\"%s\",method=\"%s\",status_class=\"%s\"",
			httpSurface(r.URL.Path), httpMethod(r.Method), statusClass(recorder.status))
		m.increment("kave_v2_http_requests_total", labels)
		m.observe("kave_v2_http_request_duration_seconds", labels, time.Since(started))
	})
}

func httpSurface(path string) string {
	switch {
	case path == "/livez" || path == "/readyz":
		return "health"
	case path == "/metrics":
		return "metrics"
	case strings.HasPrefix(path, "/v2/agents/"):
		return "gateway"
	case strings.HasPrefix(path, "/kave.kernel.v2.KernelService/"):
		return "control"
	default:
		return "ui"
	}
}

func httpMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost:
		return strings.ToLower(method)
	default:
		return "other"
	}
}

func statusClass(status int) string {
	if status < 100 || status > 599 {
		return "other"
	}
	return fmt.Sprintf("%dxx", status/100)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(p []byte) (int, error) {
	return w.ResponseWriter.Write(p)
}

func (w *statusRecorder) Flush() {
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *statusRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// WrapAuthenticator records authentication latency and a fixed outcome while
// ensuring the raw service key never enters logs or metrics.
func (m *Metrics) WrapAuthenticator(next httpapi.Authenticator) httpapi.Authenticator {
	return observedAuthenticator{next: next, metrics: m}
}

type observedAuthenticator struct {
	next    httpapi.Authenticator
	metrics *Metrics
}

func (a observedAuthenticator) AuthenticateRaw(ctx context.Context, raw string) (httpapi.Identity, error) {
	started := time.Now()
	identity, err := a.next.AuthenticateRaw(ctx, raw)
	result := "success"
	if errors.Is(err, v2postgres.ErrInvalidServiceKey) {
		result = "invalid"
	} else if err != nil {
		result = "unavailable"
	}
	labels := "result=\"" + result + "\""
	a.metrics.increment("kave_v2_auth_attempts_total", labels)
	a.metrics.observe("kave_v2_auth_duration_seconds", labels, time.Since(started))
	return identity, err
}

// WrapAdmissionStore measures the atomic Consume boundary without recording
// request scope or limit identifiers.
func (m *Metrics) WrapAdmissionStore(next corev2.AdmissionStore) corev2.AdmissionStore {
	return observedAdmissionStore{next: next, metrics: m}
}

type observedAdmissionStore struct {
	next    corev2.AdmissionStore
	metrics *Metrics
}

func (s observedAdmissionStore) Consume(ctx context.Context, req corev2.ConsumeRequest) (corev2.Decision, error) {
	started := time.Now()
	decision, err := s.next.Consume(ctx, req)
	result := "admitted"
	if decision.Status == corev2.DecisionRejected || errors.Is(err, corev2.ErrLimitExceeded) {
		result = "rejected"
	} else if err != nil {
		result = "error"
	}
	labels := "result=\"" + result + "\""
	s.metrics.increment("kave_v2_admission_decisions_total", labels)
	s.metrics.observe("kave_v2_admission_duration_seconds", labels, time.Since(started))
	return decision, err
}

// WrapProviderStore instruments the provider accounting state machine. Begin
// includes bounded orphan recovery, so non-domain Begin failures are also
// counted as admission-or-recovery accounting failures.
func (m *Metrics) WrapProviderStore(next provider.Store) provider.Store {
	return observedProviderStore{next: next, metrics: m}
}

// WrapProviderRouteValidator reports only the fixed validation outcome. The
// route, model, provider, credential, and provider request ID never become
// labels, so operators can alert on activation failures without expanding the
// metrics cardinality or exporting tenant configuration.
func (m *Metrics) WrapProviderRouteValidator(next corev2.ProviderRouteValidator) corev2.ProviderRouteValidator {
	return observedProviderRouteValidator{next: next, metrics: m}
}

type observedProviderRouteValidator struct {
	next    corev2.ProviderRouteValidator
	metrics *Metrics
}

func (v observedProviderRouteValidator) ValidateProviderRoute(ctx context.Context, target corev2.ProviderRouteValidationTarget) (corev2.ProviderRouteValidationEvidence, error) {
	started := time.Now()
	evidence, err := v.next.ValidateProviderRoute(ctx, target)
	result := "success"
	if errors.Is(err, corev2.ErrProviderValidationFailed) {
		result = "rejected"
	} else if err != nil {
		result = "error"
	}
	labels := "result=\"" + result + "\""
	v.metrics.increment("kave_v2_provider_validations_total", labels)
	v.metrics.observe("kave_v2_provider_validation_duration_seconds", labels, time.Since(started))
	return evidence, err
}

type observedProviderStore struct {
	next    provider.Store
	metrics *Metrics
}

func (s observedProviderStore) Begin(ctx context.Context, req provider.BeginRequest) (provider.Grant, error) {
	started := time.Now()
	grant, err := s.next.Begin(ctx, req)
	result := providerResult(err)
	s.observe("begin", result, time.Since(started))
	if err != nil && result == "error" {
		s.metrics.increment("kave_v2_accounting_failures_total", "phase=\"admission_or_recovery\"")
	}
	return grant, err
}

func (s observedProviderStore) StartAttempt(ctx context.Context, req provider.AttemptRequest) error {
	started := time.Now()
	err := s.next.StartAttempt(ctx, req)
	s.observe("start_attempt", providerResult(err), time.Since(started))
	return err
}

func (s observedProviderStore) RenewLease(ctx context.Context, grant provider.Grant) error {
	started := time.Now()
	err := s.next.RenewLease(ctx, grant)
	s.observe("renew_lease", providerResult(err), time.Since(started))
	return err
}

func (s observedProviderStore) Complete(ctx context.Context, req provider.CompleteRequest) error {
	started := time.Now()
	err := s.next.Complete(ctx, req)
	result := providerResult(err)
	if err == nil {
		switch {
		case req.Uncertain:
			result = "uncertain"
		case req.Usage.Reported:
			result = "reported"
		default:
			result = "unreported"
		}
	}
	s.observe("settle", result, time.Since(started))
	if err != nil {
		s.metrics.increment("kave_v2_accounting_failures_total", "phase=\"settlement\"")
	}
	return err
}

func (s observedProviderStore) observe(operation, result string, elapsed time.Duration) {
	labels := fmt.Sprintf("operation=\"%s\",result=\"%s\"", operation, result)
	s.metrics.increment("kave_v2_provider_operations_total", labels)
	s.metrics.observe("kave_v2_provider_operation_duration_seconds", labels, elapsed)
}

func providerResult(err error) string {
	if err == nil {
		return "success"
	}
	switch {
	case errors.Is(err, corev2.ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, corev2.ErrLimitExceeded), errors.Is(err, provider.ErrReservationUnavailable):
		return "rejected"
	case errors.Is(err, provider.ErrRouteUnavailable), errors.Is(err, provider.ErrUnsupportedEndpoint):
		return "unavailable"
	case errors.Is(err, provider.ErrAlreadyInvoked), errors.Is(err, provider.ErrInvocationInProgress), errors.Is(err, corev2.ErrIdempotencyConflict):
		return "conflict"
	default:
		return "error"
	}
}
