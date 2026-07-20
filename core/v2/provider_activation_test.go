package v2

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type activationStoreStub struct {
	target         ProviderRouteValidationTarget
	result         ProviderRouteActivationResult
	prepareErr     error
	recordErr      error
	prepareCalls   int
	recordCalls    int
	recordedReq    ActivateProviderRouteRequest
	recordedTarget ProviderRouteValidationTarget
	evidence       ProviderRouteValidationEvidence
	success        bool
}

func (s *activationStoreStub) PrepareProviderRouteActivation(_ context.Context, req ActivateProviderRouteRequest) (ProviderRouteValidationTarget, error) {
	s.prepareCalls++
	s.recordedReq = req
	return s.target, s.prepareErr
}

func (s *activationStoreStub) RecordProviderRouteValidation(_ context.Context, req ActivateProviderRouteRequest, target ProviderRouteValidationTarget, evidence ProviderRouteValidationEvidence, success bool) (ProviderRouteActivationResult, error) {
	s.recordCalls++
	s.recordedReq, s.recordedTarget, s.evidence, s.success = req, target, evidence, success
	result := s.result
	if result.Status == "" {
		result.Status = map[bool]string{true: "active", false: "invalid"}[success]
	}
	return result, s.recordErr
}

type activationValidatorStub struct {
	evidence ProviderRouteValidationEvidence
	err      error
	calls    int
	target   ProviderRouteValidationTarget
}

func (s *activationValidatorStub) ValidateProviderRoute(_ context.Context, target ProviderRouteValidationTarget) (ProviderRouteValidationEvidence, error) {
	s.calls++
	s.target = target
	s.target.Credential = append([]byte(nil), target.Credential...)
	if s.evidence.HTTPStatus == 0 {
		s.evidence.HTTPStatus = 200
	}
	return s.evidence, s.err
}

func TestProviderRouteActivationRequiresConfigAuthority(t *testing.T) {
	t.Parallel()
	req := validActivationRequest()
	req.Caller.Operations = []Operation{OperationSecretsWrite}
	if err := req.Validate(); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Validate() error = %v, want unauthorized", err)
	}
	req = validActivationRequest()
	req.Caller.Bootstrap = true
	req.Caller.NamespaceID = ""
	if err := req.Validate(); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("bootstrap Validate() error = %v, want unauthorized", err)
	}
}

func TestProviderRouteActivationRequestAndTargetAreStructurallyBounded(t *testing.T) {
	t.Parallel()
	request := validActivationRequest()
	request.NamespaceID = "namespace/other"
	if err := request.Validate(); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("cross-namespace request error = %v", err)
	}
	request = validActivationRequest()
	request.Route = "route/with/slash"
	if err := request.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("unsafe route name error = %v", err)
	}

	valid := validActivationTarget()
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ProviderRouteValidationTarget){
		"route revision": func(target *ProviderRouteValidationTarget) { target.RouteRevision = 0 },
		"secret version": func(target *ProviderRouteValidationTarget) { target.SecretVersion = 0 },
		"protocol":       func(target *ProviderRouteValidationTarget) { target.Protocol = "" },
		"base URL":       func(target *ProviderRouteValidationTarget) { target.BaseURL = "" },
		"credential":     func(target *ProviderRouteValidationTarget) { target.Credential = nil },
		"reference":      func(target *ProviderRouteValidationTarget) { target.SecretID = "secret\nforged" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			target := validActivationTarget()
			mutate(&target)
			if err := target.Validate(); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("Validate() error = %v, want invalid argument", err)
			}
		})
	}

	if !ModelAllowed("gpt-safe", []string{"gpt-safe"}) || ModelAllowed("gpt", []string{"gpt-safe"}) || ModelAllowed("", []string{""}) {
		t.Fatal("ModelAllowed did not enforce an exact, non-empty match")
	}
}

func TestProviderRouteActivationRecordsSuccessAndClearsCredential(t *testing.T) {
	t.Parallel()
	validatedAt := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	store := &activationStoreStub{
		target: validActivationTarget(),
		result: ProviderRouteActivationResult{
			RouteID: "route/openai", Route: "openai", Provider: "openai", Model: "gpt-5-mini",
			Status: "active", RouteRevision: 2, SecretVersion: 1, ValidatedAt: validatedAt,
			ProviderRequestID: "provider-request-123",
		},
	}
	validator := &activationValidatorStub{evidence: ProviderRouteValidationEvidence{
		HTTPStatus: 200, ProviderRequestID: "provider-request-123",
	}}
	service := NewProviderRouteActivationService(store, validator)
	result, err := service.Activate(context.Background(), validActivationRequest())
	if err != nil || store.prepareCalls != 1 || store.recordCalls != 1 || validator.calls != 1 || !store.success ||
		result.Status != "active" || result.ProviderRequestID != "provider-request-123" || !result.ValidatedAt.Equal(validatedAt) {
		t.Fatalf("Activate() = %+v, %v; prepare=%d record=%d validate=%d success=%v", result, err, store.prepareCalls, store.recordCalls, validator.calls, store.success)
	}
	if store.evidence != validator.evidence || !reflect.DeepEqual(store.recordedReq, validActivationRequest()) ||
		store.recordedTarget.RouteRevision != 1 || store.recordedTarget.SecretVersion != 1 ||
		string(validator.target.Credential) != "secret" {
		t.Fatalf("recorded request=%+v target=%+v evidence=%+v validator target=%+v", store.recordedReq, store.recordedTarget, store.evidence, validator.target)
	}
	for _, value := range store.target.Credential {
		// Prepare returns a slice by value; clearing the returned target must clear
		// the same backing array owned by the store stub.
		if value != 0 {
			t.Fatal("credential was not cleared")
		}
	}
}

func TestProviderRouteActivationFailsClosedAndRecordsFailure(t *testing.T) {
	t.Parallel()
	credential := []byte("secret")
	store := &activationStoreStub{target: validActivationTarget()}
	store.target.Credential = credential
	validator := &activationValidatorStub{
		evidence: ProviderRouteValidationEvidence{HTTPStatus: 401, ProviderRequestID: "provider-request-denied"},
		err:      errors.New("upstream rejected"),
	}
	service := NewProviderRouteActivationService(store, validator)
	_, err := service.Activate(context.Background(), validActivationRequest())
	if !errors.Is(err, ErrProviderValidationFailed) || store.recordCalls != 1 || store.success ||
		store.evidence.HTTPStatus != 401 || store.evidence.ProviderRequestID != "provider-request-denied" {
		t.Fatalf("Activate() error = %v; records=%d success=%v evidence=%+v", err, store.recordCalls, store.success, store.evidence)
	}
	for _, value := range credential {
		if value != 0 {
			t.Fatal("credential was not cleared")
		}
	}
}

func TestProviderRouteActivationDoesNotValidateAfterPrepareFailure(t *testing.T) {
	t.Parallel()
	prepareErr := errors.New("route unavailable")
	store := &activationStoreStub{prepareErr: prepareErr}
	validator := &activationValidatorStub{}
	service := NewProviderRouteActivationService(store, validator)

	_, err := service.Activate(context.Background(), validActivationRequest())
	if !errors.Is(err, prepareErr) || store.prepareCalls != 1 || store.recordCalls != 0 || validator.calls != 0 {
		t.Fatalf("Activate() error=%v prepare=%d record=%d validate=%d", err, store.prepareCalls, store.recordCalls, validator.calls)
	}
}

func TestProviderRouteActivationClearsCredentialWhenRecordingFails(t *testing.T) {
	t.Parallel()
	recordErr := errors.New("activation state changed")
	credential := []byte("secret")
	store := &activationStoreStub{target: validActivationTarget(), recordErr: recordErr}
	store.target.Credential = credential
	validator := &activationValidatorStub{}

	_, err := NewProviderRouteActivationService(store, validator).Activate(context.Background(), validActivationRequest())
	if !errors.Is(err, recordErr) || store.recordCalls != 1 {
		t.Fatalf("Activate() error=%v record=%d", err, store.recordCalls)
	}
	for _, value := range credential {
		if value != 0 {
			t.Fatal("credential was not cleared after record failure")
		}
	}
}

func validActivationRequest() ActivateProviderRouteRequest {
	return ActivateProviderRouteRequest{
		Caller:      Caller{AccountID: "account/acme", NamespaceID: "namespace/prod", ServiceKeyID: "key/admin", Operations: []Operation{OperationConfigApply}},
		NamespaceID: "namespace/prod", Route: "openai", Model: "gpt-5-mini",
	}
}

func validActivationTarget() ProviderRouteValidationTarget {
	return ProviderRouteValidationTarget{
		AccountID: "account/acme", NamespaceID: "namespace/prod", RouteID: "route/openai", Route: "openai",
		RouteRevision: 1, Provider: "openai", Protocol: "openai", BaseURL: "https://api.openai.com/v1",
		Model: "gpt-5-mini", SecretID: "secret/openai", SecretName: "openai-key", SecretVersion: 1,
		Credential: []byte("secret"),
	}
}
