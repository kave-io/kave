package gateway

import (
	"context"
	"errors"
	"net/http"
	"testing"

	corev2 "github.com/kave-io/kave/core/v2"
	"github.com/kave-io/kave/server/internal/v2/provider"
)

type validationAdapterStub struct {
	target   provider.ValidationTarget
	evidence provider.ValidationEvidence
	err      error
	calls    int
}

func (*validationAdapterStub) Protocol() string { return "openai" }

func (*validationAdapterStub) ApplyAuthentication(http.Header, []byte) error { return nil }

func (s *validationAdapterStub) Validate(_ context.Context, _ provider.HTTPDoer, target provider.ValidationTarget) (provider.ValidationEvidence, error) {
	s.calls++
	s.target = target
	s.target.Credential = append([]byte(nil), target.Credential...)
	return s.evidence, s.err
}

func (*validationAdapterStub) ParseUsage([]byte, string, string) (provider.Usage, error) {
	return provider.Usage{}, nil
}

type validationRoundTripperStub struct{}

func (validationRoundTripperStub) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("validation adapter unexpectedly used HTTP transport")
}

func TestRouteValidatorPreservesBoundedProviderEvidence(t *testing.T) {
	t.Parallel()
	adapter := &validationAdapterStub{evidence: provider.ValidationEvidence{
		HTTPStatus: 200, ProviderRequestID: "provider-request-123",
	}}
	registry, err := provider.NewRegistry(map[string]provider.Adapter{"openai": adapter}, map[string]provider.Adapter{"openai": adapter})
	if err != nil {
		t.Fatal(err)
	}
	validator, err := newRouteValidatorWithTransport(validationRoundTripperStub{}, registry)
	if err != nil {
		t.Fatal(err)
	}
	target := validProviderValidationTarget()

	evidence, err := validator.ValidateProviderRoute(context.Background(), target)
	if err != nil || evidence.HTTPStatus != 200 || evidence.ProviderRequestID != "provider-request-123" || adapter.calls != 1 {
		t.Fatalf("ValidateProviderRoute() = %+v, %v; calls=%d", evidence, err, adapter.calls)
	}
	if adapter.target.Provider != "openai" || adapter.target.Protocol != "openai" ||
		adapter.target.BaseURL != target.BaseURL || adapter.target.Model != "gpt-safe" ||
		string(adapter.target.Credential) != "provider-secret" {
		t.Fatalf("adapter target = %+v", adapter.target)
	}
}

func TestRouteValidatorFailsClosedAndRetainsFailureEvidence(t *testing.T) {
	t.Parallel()
	adapter := &validationAdapterStub{
		evidence: provider.ValidationEvidence{HTTPStatus: 401, ProviderRequestID: "provider-request-denied"},
		err:      errors.New("upstream denied validation"),
	}
	registry, err := provider.NewRegistry(map[string]provider.Adapter{"openai": adapter}, map[string]provider.Adapter{"openai": adapter})
	if err != nil {
		t.Fatal(err)
	}
	validator, err := newRouteValidatorWithTransport(validationRoundTripperStub{}, registry)
	if err != nil {
		t.Fatal(err)
	}

	evidence, err := validator.ValidateProviderRoute(context.Background(), validProviderValidationTarget())
	if !errors.Is(err, corev2.ErrProviderValidationFailed) || evidence.HTTPStatus != 401 ||
		evidence.ProviderRequestID != "provider-request-denied" || adapter.calls != 1 {
		t.Fatalf("ValidateProviderRoute() = %+v, %v; calls=%d", evidence, err, adapter.calls)
	}
}

func TestRouteValidatorRejectsInvalidTargetBeforeAdapter(t *testing.T) {
	t.Parallel()
	adapter := &validationAdapterStub{}
	registry, err := provider.NewRegistry(map[string]provider.Adapter{"openai": adapter}, map[string]provider.Adapter{"openai": adapter})
	if err != nil {
		t.Fatal(err)
	}
	validator, err := newRouteValidatorWithTransport(validationRoundTripperStub{}, registry)
	if err != nil {
		t.Fatal(err)
	}
	target := validProviderValidationTarget()
	target.Credential = nil

	if _, err := validator.ValidateProviderRoute(context.Background(), target); !errors.Is(err, corev2.ErrInvalidArgument) || adapter.calls != 0 {
		t.Fatalf("ValidateProviderRoute() error=%v calls=%d", err, adapter.calls)
	}
}

func validProviderValidationTarget() corev2.ProviderRouteValidationTarget {
	return corev2.ProviderRouteValidationTarget{
		AccountID: "account/acme", NamespaceID: "namespace/prod", RouteID: "route/openai", Route: "openai",
		RouteRevision: 3, Provider: "openai", Protocol: "openai", BaseURL: "https://api.openai.com/v1",
		Model: "gpt-safe", SecretID: "secret/openai", SecretName: "openai-key", SecretVersion: 2,
		Credential: []byte("provider-secret"),
	}
}
