package v2_test

import (
	"errors"
	"testing"

	v2 "github.com/kave-io/kave/core/v2"
)

func TestScopeValidationRejectsHeaderInjection(t *testing.T) {
	t.Parallel()

	for _, scope := range []v2.Scope{
		{Tenant: "clinic/good\r\nX-Evil: yes"},
		{Actor: " user/a"},
		{BillTo: "user/a b"},
	} {
		if err := scope.Validate(); !errors.Is(err, v2.ErrInvalidArgument) {
			t.Fatalf("Validate() error = %v, want invalid argument", err)
		}
	}
}

func TestAdmissionScopeRequiresTenantAndBillingSubject(t *testing.T) {
	t.Parallel()

	for _, scope := range []v2.Scope{
		{},
		{Tenant: "clinic/abc"},
		{BillTo: "clinic/abc"},
	} {
		if err := scope.ValidateAdmission(); !errors.Is(err, v2.ErrInvalidArgument) {
			t.Fatalf("ValidateAdmission(%+v) error = %v, want invalid argument", scope, err)
		}
	}
	if err := (v2.Scope{Tenant: "system/indexer", BillTo: "system/indexer"}).ValidateAdmission(); err != nil {
		t.Fatalf("ValidateAdmission() error = %v", err)
	}
}

func TestManifestValidation(t *testing.T) {
	t.Parallel()

	manifest := v2.Manifest{
		Namespace: v2.Namespace{Account: "account/acme", Application: "simorq", Environment: "production"},
		Routes: []v2.RouteSpec{{
			Name: "openai-primary", Provider: "openai", Secret: "openai-primary",
			BaseURL: "https://api.openai.com", AllowedModels: []string{"gpt-5-mini"}, DefaultModel: "gpt-5-mini",
			PricingRevision: 1, Pricing: []v2.ModelPrice{{Model: "gpt-5-mini"}},
		}},
		Agents: []v2.AgentSpec{{Name: "clinic-assistant", Kind: v2.AgentLLM, Route: "openai-primary", Enabled: true}},
		Limits: []v2.LimitSpec{{
			Key: "clinic-actions", Metric: "ai_actions", Window: v2.WindowMonth, HardCap: 100,
			Selector: v2.LimitSelector{Agent: "clinic-assistant"}, Enabled: true,
		}},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid manifest: %v", err)
	}

	manifest.Routes[0].BaseURL = "http://api.openai.com"
	if err := manifest.Validate(); !errors.Is(err, v2.ErrInvalidArgument) {
		t.Fatalf("plain remote HTTP error = %v, want invalid argument", err)
	}
}

func TestConsumeValidationChecksCapabilities(t *testing.T) {
	t.Parallel()

	req := consumeRequest()
	req.Caller.AllowedAgents = []v2.Ref{"other-agent"}
	if err := req.Validate(); !errors.Is(err, v2.ErrUnauthorized) {
		t.Fatalf("agent capability error = %v, want unauthorized", err)
	}

	req = consumeRequest()
	req.Caller.CanAssertScope = false
	if err := req.Validate(); !errors.Is(err, v2.ErrUnauthorized) {
		t.Fatalf("scope capability error = %v, want unauthorized", err)
	}

	req = consumeRequest()
	req.Caller.AllowedAgents = nil
	if err := req.Validate(); !errors.Is(err, v2.ErrUnauthorized) {
		t.Fatalf("empty agent capability error = %v, want unauthorized", err)
	}

	req = consumeRequest()
	req.Caller.Operations = nil
	if err := req.Validate(); !errors.Is(err, v2.ErrUnauthorized) {
		t.Fatalf("empty operation capability error = %v, want unauthorized", err)
	}
}

func consumeRequest() v2.ConsumeRequest {
	return v2.ConsumeRequest{
		Caller: v2.Caller{
			AccountID: "account/acme", NamespaceID: "namespace/prod", ServiceKeyID: "key/worker",
			Operations: []v2.Operation{v2.OperationConsume}, AllowedAgents: []v2.Ref{"clinic-assistant"}, CanAssertScope: true,
		},
		Agent:  "clinic-assistant",
		Scope:  v2.Scope{Tenant: "clinic/abc", Actor: "user/def", BillTo: "clinic/abc", Session: "run/123", Feature: "ai_actions"},
		Metric: "ai_actions", Units: 1, IdempotencyKey: "run/123",
	}
}
