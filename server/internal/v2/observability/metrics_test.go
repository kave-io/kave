package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev2 "github.com/kave-io/kave/core/v2"
	"github.com/kave-io/kave/server/internal/v2/httpapi"
	v2postgres "github.com/kave-io/kave/server/internal/v2/postgres"
	"github.com/kave-io/kave/server/internal/v2/provider"
)

type authFunc func(context.Context, string) (httpapi.Identity, error)

func (f authFunc) AuthenticateRaw(ctx context.Context, raw string) (httpapi.Identity, error) {
	return f(ctx, raw)
}

type admissionFunc func(context.Context, corev2.ConsumeRequest) (corev2.Decision, error)

func (f admissionFunc) Consume(ctx context.Context, req corev2.ConsumeRequest) (corev2.Decision, error) {
	return f(ctx, req)
}

type routeValidatorFunc func(context.Context, corev2.ProviderRouteValidationTarget) (corev2.ProviderRouteValidationEvidence, error)

func (f routeValidatorFunc) ValidateProviderRoute(ctx context.Context, target corev2.ProviderRouteValidationTarget) (corev2.ProviderRouteValidationEvidence, error) {
	return f(ctx, target)
}

type providerStore struct {
	begin    func(context.Context, provider.BeginRequest) (provider.Grant, error)
	complete func(context.Context, provider.CompleteRequest) error
}

func (s providerStore) Begin(ctx context.Context, req provider.BeginRequest) (provider.Grant, error) {
	return s.begin(ctx, req)
}
func (providerStore) StartAttempt(context.Context, provider.AttemptRequest) error { return nil }
func (providerStore) RenewLease(context.Context, provider.Grant) error            { return nil }
func (s providerStore) Complete(ctx context.Context, req provider.CompleteRequest) error {
	return s.complete(ctx, req)
}

func TestMetricsExposeOnlyFixedLowCardinalityLabels(t *testing.T) {
	t.Parallel()
	metrics := New(func() PoolStats { return PoolStats{Acquired: 1, Idle: 2, Total: 3, Max: 8} })
	privateMarker := "must stay private"
	tenant := "tenant/medical-highly-sensitive"
	invocation := "invocation/private-id"

	auth := metrics.WrapAuthenticator(authFunc(func(context.Context, string) (httpapi.Identity, error) {
		return httpapi.Identity{}, v2postgres.ErrInvalidServiceKey
	}))
	_, _ = auth.AuthenticateRaw(context.Background(), privateMarker)

	admission := metrics.WrapAdmissionStore(admissionFunc(func(context.Context, corev2.ConsumeRequest) (corev2.Decision, error) {
		return corev2.Decision{InvocationID: invocation, Status: corev2.DecisionRejected}, corev2.ErrLimitExceeded
	}))
	_, _ = admission.Consume(context.Background(), corev2.ConsumeRequest{Scope: corev2.Scope{Tenant: corev2.Ref(tenant)}})

	providerMetrics := metrics.WrapProviderStore(providerStore{
		begin: func(context.Context, provider.BeginRequest) (provider.Grant, error) {
			return provider.Grant{}, errors.New("database failed for " + tenant)
		},
		complete: func(context.Context, provider.CompleteRequest) error { return nil },
	})
	_, _ = providerMetrics.Begin(context.Background(), provider.BeginRequest{})
	_ = providerMetrics.Complete(context.Background(), provider.CompleteRequest{Uncertain: true})
	validator := metrics.WrapProviderRouteValidator(routeValidatorFunc(func(context.Context, corev2.ProviderRouteValidationTarget) (corev2.ProviderRouteValidationEvidence, error) {
		return corev2.ProviderRouteValidationEvidence{}, corev2.ErrProviderValidationFailed
	}))
	_, _ = validator.ValidateProviderRoute(context.Background(), corev2.ProviderRouteValidationTarget{Route: corev2.Ref(tenant)})

	recorder := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := recorder.Body.String()
	for _, forbidden := range []string{privateMarker, tenant, invocation, "model=", "route=", "service_key="} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("metrics leaked %q:\n%s", forbidden, body)
		}
	}
	for _, required := range []string{
		`kave_v2_auth_attempts_total{result="invalid"} 1`,
		`kave_v2_admission_decisions_total{result="rejected"} 1`,
		`kave_v2_accounting_failures_total{phase="admission_or_recovery"} 1`,
		`kave_v2_provider_operations_total{operation="settle",result="uncertain"} 1`,
		`kave_v2_provider_validations_total{result="rejected"} 1`,
		`kave_v2_postgres_connections{state="max"} 8`,
	} {
		if !strings.Contains(body, required) {
			t.Errorf("metrics missing %q:\n%s", required, body)
		}
	}
}

func TestHTTPMetricsHaveBoundedSurfaceAndPreserveStreaming(t *testing.T) {
	t.Parallel()
	metrics := New(nil)
	handler := metrics.HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		if _, err := w.Write([]byte("ok")); err != nil {
			t.Error(err)
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("Flush(): %v", err)
		}
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v2/agents/private-agent/openai/responses", nil))
	if recorder.Code != http.StatusCreated || recorder.Body.String() != "ok" || !recorder.Flushed {
		t.Fatalf("response = %d/%q flushed=%v", recorder.Code, recorder.Body.String(), recorder.Flushed)
	}

	metricsRecorder := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(metricsRecorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := metricsRecorder.Body.String()
	if strings.Contains(body, "private-agent") || !strings.Contains(body, `surface="gateway",method="post",status_class="2xx"`) {
		t.Fatalf("unexpected HTTP metrics:\n%s", body)
	}
}
