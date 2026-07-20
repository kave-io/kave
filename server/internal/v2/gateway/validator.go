package gateway

import (
	"context"
	"errors"
	"net/http"

	corev2 "github.com/kave-io/kave/core/v2"
	"github.com/kave-io/kave/server/internal/v2/provider"
)

// RouteValidator performs the explicit, payload-free provider activation probe
// over the exact same DNS-pinning, redirect-denying egress boundary as normal
// gateway invocations.
type RouteValidator struct {
	client   *providerHTTPClient
	adapters *provider.Registry
}

func NewRouteValidator(policy ProviderEgressPolicy) (*RouteValidator, error) {
	transport, err := NewProviderTransport(policy)
	if err != nil {
		return nil, err
	}
	return newRouteValidatorWithTransport(transport, provider.DefaultRegistry())
}

func newRouteValidatorWithTransport(transport http.RoundTripper, adapters *provider.Registry) (*RouteValidator, error) {
	if transport == nil || adapters == nil {
		return nil, errors.New("v2 gateway: provider validation transport and adapters are required")
	}
	return &RouteValidator{client: newProviderHTTPClient(transport), adapters: adapters}, nil
}

func (v *RouteValidator) ValidateProviderRoute(ctx context.Context, target corev2.ProviderRouteValidationTarget) (corev2.ProviderRouteValidationEvidence, error) {
	if v == nil || v.client == nil || v.adapters == nil {
		return corev2.ProviderRouteValidationEvidence{}, corev2.ErrProviderValidationFailed
	}
	if err := target.Validate(); err != nil {
		return corev2.ProviderRouteValidationEvidence{}, err
	}
	adapter, err := v.adapters.Resolve(string(target.Provider), target.Protocol)
	if err != nil {
		return corev2.ProviderRouteValidationEvidence{}, corev2.ErrProviderValidationFailed
	}
	evidence, err := adapter.Validate(ctx, v.client, provider.ValidationTarget{
		Provider: string(target.Provider), Protocol: target.Protocol, BaseURL: target.BaseURL,
		Model: string(target.Model), Credential: target.Credential,
	})
	result := corev2.ProviderRouteValidationEvidence{
		HTTPStatus: evidence.HTTPStatus, ProviderRequestID: evidence.ProviderRequestID,
	}
	if err != nil {
		return result, corev2.ErrProviderValidationFailed
	}
	return result, nil
}

var _ corev2.ProviderRouteValidator = (*RouteValidator)(nil)
