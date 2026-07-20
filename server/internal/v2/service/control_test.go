package service_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	corev2 "github.com/kave-io/kave/core/v2"
	kernelv2 "github.com/kave-io/kave/proto/gen/kave/kernel/v2"
	"github.com/kave-io/kave/server/internal/v2/authctx"
	"github.com/kave-io/kave/server/internal/v2/service"
)

type applyStoreFunc func(context.Context, corev2.ApplyRequest) (corev2.ApplyResult, error)

func (f applyStoreFunc) Apply(ctx context.Context, req corev2.ApplyRequest) (corev2.ApplyResult, error) {
	return f(ctx, req)
}

type secretStoreFake struct {
	plaintext []byte
}

func (f *secretStoreFake) PutSecret(_ context.Context, req corev2.PutSecretRequest) (corev2.SecretMetadata, error) {
	f.plaintext = bytes.Clone(req.Plaintext)
	return corev2.SecretMetadata{ID: "sec_one", Name: req.Name, Source: req.Source(), Version: 1, Status: "active", UpdatedAt: 123}, nil
}

func (*secretStoreFake) RevokeSecret(context.Context, corev2.RevokeSecretRequest) error { return nil }

type keyStoreFake struct {
	issued corev2.IssueServiceKeyRequest
}

type limitSyncStoreFake struct {
	req corev2.SyncLimitsRequest
	err error
}

type routeActivationStoreFake struct {
	target   corev2.ProviderRouteValidationTarget
	result   corev2.ProviderRouteActivationResult
	request  corev2.ActivateProviderRouteRequest
	evidence corev2.ProviderRouteValidationEvidence
	success  bool
	recorded bool
}

func (f *routeActivationStoreFake) PrepareProviderRouteActivation(_ context.Context, req corev2.ActivateProviderRouteRequest) (corev2.ProviderRouteValidationTarget, error) {
	f.request = req
	return f.target, nil
}

func (f *routeActivationStoreFake) RecordProviderRouteValidation(_ context.Context, req corev2.ActivateProviderRouteRequest, _ corev2.ProviderRouteValidationTarget, evidence corev2.ProviderRouteValidationEvidence, success bool) (corev2.ProviderRouteActivationResult, error) {
	f.request, f.evidence, f.success, f.recorded = req, evidence, success, true
	return f.result, nil
}

type routeActivationValidatorFake struct {
	evidence corev2.ProviderRouteValidationEvidence
	err      error
}

func (f routeActivationValidatorFake) ValidateProviderRoute(context.Context, corev2.ProviderRouteValidationTarget) (corev2.ProviderRouteValidationEvidence, error) {
	return f.evidence, f.err
}

func (f *limitSyncStoreFake) SyncLimits(_ context.Context, req corev2.SyncLimitsRequest) (corev2.SyncLimitsResult, error) {
	f.req = req
	if f.err != nil {
		return corev2.SyncLimitsResult{}, f.err
	}
	return corev2.SyncLimitsResult{Revision: req.Revision, Created: 2, Updated: 1, Disabled: 3}, nil
}

func (f *keyStoreFake) IssueServiceKey(_ context.Context, req corev2.IssueServiceKeyRequest) (corev2.IssuedServiceKey, error) {
	f.issued = req
	return corev2.IssuedServiceKey{
		ID: "key_worker", Name: req.Name, Prefix: corev2.RawServiceKeyPrefix + req.LookupPrefix,
		Created: true, CreatedAt: time.Unix(1_700_000_000, 0).UTC(),
	}, nil
}

func (*keyStoreFake) RevokeServiceKey(context.Context, corev2.RevokeServiceKeyRequest) error {
	return nil
}

func TestApplyAcceptsOnlyAccountScopedBootstrapManifest(t *testing.T) {
	t.Parallel()
	var captured corev2.ApplyRequest
	apply := corev2.NewApplyService(applyStoreFunc(func(_ context.Context, req corev2.ApplyRequest) (corev2.ApplyResult, error) {
		captured = req
		return corev2.ApplyResult{NamespaceID: "namespace/prod", Revision: 1, Applied: true}, nil
	}))
	server := service.New(nil, service.WithApply(apply))
	ctx := authctx.WithCaller(context.Background(), bootstrapCaller())
	request := &kernelv2.ApplyRequest{
		Manifest: &kernelv2.Manifest{
			Namespace: &kernelv2.NamespaceSpec{Account: "account/acme", Application: "simorq", Environment: "production"},
			Routes: []*kernelv2.RouteSpec{{
				Name: "openai", Provider: "openai", Secret: "openai", AllowedModels: []string{"gpt-5"},
				DefaultModel: "gpt-5", PricingRevision: 3,
				Pricing: []*kernelv2.ModelPrice{{Model: "gpt-5", InputNanosPerMillionTokens: 2, OutputNanosPerMillionTokens: 8}},
			}},
			Agents: []*kernelv2.AgentSpec{{Name: "assistant", Kind: kernelv2.AgentKind_AGENT_KIND_LLM, Route: "openai", Enabled: true}},
		},
		IdempotencyKey: "deploy/1",
	}
	response, err := server.Apply(ctx, connect.NewRequest(request))
	if err != nil {
		t.Fatal(err)
	}
	if response.Msg.GetNamespaceId() != "namespace/prod" || captured.Caller.AccountID != "account/acme" || !captured.Caller.Bootstrap ||
		len(captured.Manifest.Routes) != 1 || len(captured.Manifest.Routes[0].Pricing) != 1 || captured.Manifest.Routes[0].Pricing[0].OutputNanosPerMillionTokens != 8 {
		t.Fatalf("response/captured = %+v / %+v", response.Msg, captured)
	}

	request.Manifest.Namespace.Account = "account/other"
	if _, err := server.Apply(ctx, connect.NewRequest(request)); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("cross-account apply code = %v (err=%v)", connect.CodeOf(err), err)
	}
}

func TestPutSecretClearsTransportPlaintext(t *testing.T) {
	t.Parallel()
	store := &secretStoreFake{}
	server := service.New(nil, service.WithSecrets(corev2.NewSecretService(store)))
	ctx := authctx.WithCaller(context.Background(), controlCaller(corev2.OperationSecretsWrite))
	raw := []byte("provider-secret")
	request := &kernelv2.PutSecretRequest{
		NamespaceId: "namespace/prod", Name: "openai", IdempotencyKey: "secret/1",
		Value: &kernelv2.PutSecretRequest_Plaintext{Plaintext: raw},
	}
	response, err := server.PutSecret(ctx, connect.NewRequest(request))
	if err != nil {
		t.Fatal(err)
	}
	if response.Msg.GetId() != "sec_one" || string(store.plaintext) != "provider-secret" {
		t.Fatalf("response/captured = %+v / %q", response.Msg, store.plaintext)
	}
	if !bytes.Equal(raw, make([]byte, len(raw))) {
		t.Fatalf("protobuf plaintext was retained: %q", raw)
	}
}

func TestActivateProviderRouteBindsCallerAndPreservesEvidence(t *testing.T) {
	t.Parallel()
	validatedAt := time.UnixMilli(1_721_475_000_123).UTC()
	store := &routeActivationStoreFake{
		target: corev2.ProviderRouteValidationTarget{
			AccountID: "account/acme", NamespaceID: "namespace/prod", RouteID: "route/openai", Route: "openai",
			RouteRevision: 3, Provider: "openai", Protocol: "openai", BaseURL: "https://api.openai.com/v1",
			Model: "gpt-safe", SecretID: "secret/openai", SecretName: "openai-key", SecretVersion: 2,
			Credential: []byte("provider-secret"),
		},
		result: corev2.ProviderRouteActivationResult{
			RouteID: "route/openai", Route: "openai", Provider: "openai", Model: "gpt-safe", Status: "active",
			RouteRevision: 4, SecretVersion: 2, ValidatedAt: validatedAt, ProviderRequestID: "provider-request-123",
		},
	}
	validator := routeActivationValidatorFake{evidence: corev2.ProviderRouteValidationEvidence{
		HTTPStatus: 200, ProviderRequestID: "provider-request-123",
	}}
	server := service.New(nil, service.WithProviderRouteActivation(corev2.NewProviderRouteActivationService(store, validator)))
	ctx := authctx.WithCaller(context.Background(), controlCaller(corev2.OperationConfigApply))

	response, err := server.ActivateProviderRoute(ctx, connect.NewRequest(&kernelv2.ActivateProviderRouteRequest{
		NamespaceId: "namespace/prod", Route: "openai", Model: "gpt-safe",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if response.Msg.GetRouteId() != "route/openai" || response.Msg.GetStatus() != "active" ||
		response.Msg.GetRouteRevision() != 4 || response.Msg.GetSecretVersion() != 2 ||
		response.Msg.GetValidatedAtMs() != validatedAt.UnixMilli() || response.Msg.GetProviderRequestId() != "provider-request-123" {
		t.Fatalf("response = %+v", response.Msg)
	}
	if !store.recorded || !store.success || store.request.Caller.ServiceKeyID != "key_admin" ||
		store.request.NamespaceID != "namespace/prod" || store.request.Route != "openai" || store.request.Model != "gpt-safe" ||
		store.evidence.HTTPStatus != 200 || store.evidence.ProviderRequestID != "provider-request-123" {
		t.Fatalf("captured activation = request=%+v evidence=%+v success=%v recorded=%v", store.request, store.evidence, store.success, store.recorded)
	}
}

func TestActivateProviderRouteMapsValidationFailureWithoutUpstreamDetail(t *testing.T) {
	t.Parallel()
	store := &routeActivationStoreFake{
		target: corev2.ProviderRouteValidationTarget{
			AccountID: "account/acme", NamespaceID: "namespace/prod", RouteID: "route/openai", Route: "openai",
			RouteRevision: 3, Provider: "openai", Protocol: "openai", BaseURL: "https://api.openai.com/v1",
			Model: "gpt-safe", SecretID: "secret/openai", SecretName: "openai-key", SecretVersion: 2,
			Credential: []byte("provider-secret"),
		},
		result: corev2.ProviderRouteActivationResult{Status: "invalid"},
	}
	validator := routeActivationValidatorFake{
		evidence: corev2.ProviderRouteValidationEvidence{HTTPStatus: 401, ProviderRequestID: "provider-request-denied"},
		err:      errors.New("sensitive provider response"),
	}
	server := service.New(nil, service.WithProviderRouteActivation(corev2.NewProviderRouteActivationService(store, validator)))
	ctx := authctx.WithCaller(context.Background(), controlCaller(corev2.OperationConfigApply))

	_, err := server.ActivateProviderRoute(ctx, connect.NewRequest(&kernelv2.ActivateProviderRouteRequest{
		NamespaceId: "namespace/prod", Route: "openai", Model: "gpt-safe",
	}))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition || bytes.Contains([]byte(err.Error()), []byte("sensitive")) ||
		!store.recorded || store.success || store.evidence.HTTPStatus != 401 {
		t.Fatalf("error=%v code=%v evidence=%+v success=%v recorded=%v", err, connect.CodeOf(err), store.evidence, store.success, store.recorded)
	}
}

func TestIssueServiceKeyBindsRequestedNamespaceAndAgents(t *testing.T) {
	t.Parallel()
	store := &keyStoreFake{}
	server := service.New(nil, service.WithServiceKeys(corev2.NewServiceKeyService(store)))
	ctx := authctx.WithCaller(context.Background(), controlCaller(corev2.OperationKeysManage))
	material, err := corev2.GenerateServiceKeyMaterial(nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.IssueServiceKey(ctx, connect.NewRequest(&kernelv2.IssueServiceKeyRequest{
		NamespaceId: "namespace/prod", Name: "ai-worker", Operations: []string{"consume"},
		AllowedAgents: []string{"clinic-assistant"}, CanAssertScope: true, IdempotencyKey: "key/1",
		LookupPrefix: material.LookupPrefix, SecretHash: material.SecretHash[:],
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !response.Msg.GetCreated() || response.Msg.GetPrefix() != corev2.RawServiceKeyPrefix+material.LookupPrefix || response.Msg.GetCreatedAtMs() == 0 ||
		store.issued.NamespaceID != "namespace/prod" || store.issued.LookupPrefix != material.LookupPrefix ||
		!bytes.Equal(store.issued.SecretHash, material.SecretHash[:]) || len(store.issued.AllowedAgents) != 1 {
		t.Fatalf("response/captured = %+v / %+v", response.Msg, store.issued)
	}
}

func TestIssueServiceKeyRejectsMissingOrMalformedClientVerifier(t *testing.T) {
	t.Parallel()
	store := &keyStoreFake{}
	server := service.New(nil, service.WithServiceKeys(corev2.NewServiceKeyService(store)))
	ctx := authctx.WithCaller(context.Background(), controlCaller(corev2.OperationKeysManage))
	base := &kernelv2.IssueServiceKeyRequest{
		NamespaceId: "namespace/prod", Name: "worker", Operations: []string{"usage.read"}, IdempotencyKey: "key/invalid",
	}
	if _, err := server.IssueServiceKey(ctx, connect.NewRequest(base)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("missing verifier code = %v (err=%v)", connect.CodeOf(err), err)
	}
	base.LookupPrefix = "AAAAAAAAAAAAAAAAAAAAAAAA"
	base.SecretHash = make([]byte, 31)
	if _, err := server.IssueServiceKey(ctx, connect.NewRequest(base)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("short verifier code = %v (err=%v)", connect.CodeOf(err), err)
	}
	if store.issued.Name != "" {
		t.Fatalf("invalid verifier reached store: %+v", store.issued)
	}
}

func TestSyncLimitsBindsNamespaceOwnerRevisionAndCounts(t *testing.T) {
	t.Parallel()
	store := &limitSyncStoreFake{}
	server := service.New(nil, service.WithLimitSync(corev2.NewLimitSyncService(store)))
	caller := controlCaller(corev2.OperationLimitsSync)
	ctx := authctx.WithCaller(context.Background(), caller)
	response, err := server.SyncLimits(ctx, connect.NewRequest(&kernelv2.SyncLimitsRequest{
		NamespaceId: "namespace/prod", Owner: "simorq/subscriptions", Revision: 42,
		IdempotencyKey: "outbox/42",
		Limits: []*kernelv2.LimitSpec{{
			Key: "clinic/a", Metric: "ai_actions", Window: kernelv2.LimitWindow_LIMIT_WINDOW_MONTH,
			HardCap: 10, Enabled: true, Selector: &kernelv2.LimitSelector{Tenant: "clinic/a"},
		}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if response.Msg.GetRevision() != 42 || response.Msg.GetCreated() != 2 ||
		response.Msg.GetUpdated() != 1 || response.Msg.GetDisabled() != 3 {
		t.Fatalf("response = %+v", response.Msg)
	}
	if store.req.NamespaceID != "namespace/prod" || store.req.Owner != "simorq/subscriptions" ||
		store.req.Caller.ServiceKeyID != "key_admin" || len(store.req.Limits) != 1 ||
		store.req.Limits[0].Selector.Tenant != "clinic/a" {
		t.Fatalf("captured request = %+v", store.req)
	}
}

func TestSyncLimitsRejectsBootstrapAndCrossNamespaceCaller(t *testing.T) {
	t.Parallel()
	server := service.New(nil, service.WithLimitSync(corev2.NewLimitSyncService(&limitSyncStoreFake{})))
	req := connect.NewRequest(&kernelv2.SyncLimitsRequest{
		NamespaceId: "namespace/prod", Owner: "simorq/subscriptions", Revision: 1, IdempotencyKey: "outbox/1",
	})
	ctx := authctx.WithCaller(context.Background(), bootstrapCaller())
	if _, err := server.SyncLimits(ctx, req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("bootstrap code = %v (err=%v)", connect.CodeOf(err), err)
	}
	caller := corev2.Caller{
		AccountID: "account/acme", NamespaceID: "namespace/other", ServiceKeyID: "key_admin",
		Operations: []corev2.Operation{corev2.OperationLimitsSync},
	}
	ctx = authctx.WithCaller(context.Background(), caller)
	if _, err := server.SyncLimits(ctx, req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("cross-namespace code = %v (err=%v)", connect.CodeOf(err), err)
	}
}

func TestSyncLimitsMapsRevisionAndOwnershipConflicts(t *testing.T) {
	t.Parallel()
	caller := controlCaller(corev2.OperationLimitsSync)
	ctx := authctx.WithCaller(context.Background(), caller)
	req := connect.NewRequest(&kernelv2.SyncLimitsRequest{
		NamespaceId: "namespace/prod", Owner: "simorq/subscriptions", Revision: 2, IdempotencyKey: "outbox/2",
	})
	tests := []struct {
		name string
		err  error
		code connect.Code
	}{
		{name: "revision", err: &corev2.SourceRevisionConflictError{Owner: "simorq/subscriptions", Requested: 2, Current: 3}, code: connect.CodeAborted},
		{name: "ownership", err: &corev2.LimitOwnershipConflictError{Key: "operator/global"}, code: connect.CodeFailedPrecondition},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &limitSyncStoreFake{err: test.err}
			server := service.New(nil, service.WithLimitSync(corev2.NewLimitSyncService(store)))
			if _, err := server.SyncLimits(ctx, req); connect.CodeOf(err) != test.code {
				t.Fatalf("code = %v, want %v (err=%v)", connect.CodeOf(err), test.code, err)
			}
		})
	}
}

func bootstrapCaller() corev2.Caller {
	return corev2.Caller{
		AccountID: "account/acme", ServiceKeyID: "bootstrap",
		Operations: []corev2.Operation{corev2.OperationApply}, Bootstrap: true,
	}
}

func controlCaller(operations ...corev2.Operation) corev2.Caller {
	return corev2.Caller{
		AccountID: "account/acme", NamespaceID: "namespace/prod", ServiceKeyID: "key_admin",
		Operations: operations,
	}
}
