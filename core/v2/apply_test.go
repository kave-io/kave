package v2_test

import (
	"context"
	"errors"
	"testing"

	v2 "github.com/kave-io/kave/core/v2"
)

func TestApplyRequestHashIsOrderIndependent(t *testing.T) {
	t.Parallel()

	a := validApplyRequest()
	a.Manifest.Routes[0].AllowedModels = []string{"gpt-4.1", "gpt-4o", "gpt-4.1"}
	a.Manifest.Routes[0].PricingRevision = 7
	a.Manifest.Routes[0].Pricing = []v2.ModelPrice{
		{Model: "gpt-4o", InputNanosPerMillionTokens: 2, OutputNanosPerMillionTokens: 8},
		{Model: "gpt-4.1", InputNanosPerMillionTokens: 3, OutputNanosPerMillionTokens: 12},
	}
	b := a
	b.Manifest.Routes = []v2.RouteSpec{a.Manifest.Routes[1], a.Manifest.Routes[0]}
	b.Manifest.Agents = []v2.AgentSpec{a.Manifest.Agents[1], a.Manifest.Agents[0]}
	b.Manifest.Limits = []v2.LimitSpec{a.Manifest.Limits[1], a.Manifest.Limits[0]}
	b.Manifest.Routes[1].AllowedModels = []string{"gpt-4o", "gpt-4.1"}
	b.Manifest.Routes[1].Pricing = []v2.ModelPrice{a.Manifest.Routes[0].Pricing[1], a.Manifest.Routes[0].Pricing[0]}

	aHash, err := a.Hash()
	if err != nil {
		t.Fatal(err)
	}
	bHash, err := b.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if aHash != bHash {
		t.Fatalf("equivalent manifests hashed differently: %s != %s", aHash, bHash)
	}

	b.Prune = true
	bHash, err = b.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if aHash == bHash {
		t.Fatal("prune was not bound into request hash")
	}
}

func TestApplyServiceValidatesBeforeStore(t *testing.T) {
	t.Parallel()

	store := &fakeApplyStore{}
	service := v2.NewApplyService(store)
	req := validApplyRequest()
	req.Caller.AccountID = "other"
	_, err := service.Apply(context.Background(), req)
	if !errors.Is(err, v2.ErrUnauthorized) {
		t.Fatalf("Apply() error = %v, want unauthorized", err)
	}
	if store.calls != 0 {
		t.Fatalf("store called %d times for invalid request", store.calls)
	}
}

func TestApplyRequestBoundsManifestCardinality(t *testing.T) {
	t.Parallel()
	req := validApplyRequest()
	req.Manifest.Routes[0].AllowedModels = make([]string, 257)
	for i := range req.Manifest.Routes[0].AllowedModels {
		req.Manifest.Routes[0].AllowedModels[i] = "model"
	}
	if err := req.Validate(); !errors.Is(err, v2.ErrInvalidArgument) {
		t.Fatalf("Validate() error = %v, want invalid argument", err)
	}
}

func TestManifestPricingRequiresVersionAndAllowedUniqueModels(t *testing.T) {
	t.Parallel()
	req := validApplyRequest()
	req.Manifest.Routes[0].AllowedModels = []string{"gpt-4.1"}
	req.Manifest.Routes[0].DefaultModel = "gpt-4.1"
	req.Manifest.Routes[0].PricingRevision = 0
	req.Manifest.Routes[0].Pricing = []v2.ModelPrice{{Model: "gpt-4.1", InputNanosPerMillionTokens: 1}}
	if err := req.Validate(); !errors.Is(err, v2.ErrInvalidArgument) {
		t.Fatalf("unversioned pricing Validate() error = %v, want invalid argument", err)
	}
	req.Manifest.Routes[0].PricingRevision = 1
	if err := req.Validate(); err != nil {
		t.Fatalf("versioned pricing Validate() error = %v", err)
	}
	missingPrice := req
	missingPrice.Manifest.Routes = append([]v2.RouteSpec(nil), req.Manifest.Routes...)
	missingPrice.Manifest.Routes[0].Pricing = nil
	if err := missingPrice.Validate(); !errors.Is(err, v2.ErrInvalidArgument) {
		t.Fatalf("incomplete pricing Validate() error = %v, want invalid argument", err)
	}
	req.Manifest.Routes[0].Pricing = append(req.Manifest.Routes[0].Pricing, req.Manifest.Routes[0].Pricing[0])
	if err := req.Validate(); !errors.Is(err, v2.ErrInvalidArgument) {
		t.Fatalf("duplicate pricing Validate() error = %v, want invalid argument", err)
	}
	req.Manifest.Routes[0].Pricing[1].Model = "other"
	if err := req.Validate(); !errors.Is(err, v2.ErrInvalidArgument) {
		t.Fatalf("unallowed pricing Validate() error = %v, want invalid argument", err)
	}
}

func TestApplyRequestRequiresPersistedCallerIdentityUnlessBootstrap(t *testing.T) {
	t.Parallel()
	req := validApplyRequest()
	req.Caller.NamespaceID = ""
	req.Caller.ServiceKeyID = ""
	if err := req.Validate(); !errors.Is(err, v2.ErrInvalidArgument) {
		t.Fatalf("persisted caller Validate() error = %v, want invalid argument", err)
	}
	req.Caller.Bootstrap = true
	req.Caller.ServiceKeyID = "bootstrap"
	if err := req.Validate(); err != nil {
		t.Fatalf("bootstrap caller Validate() error = %v", err)
	}
}

func TestRevisionConflictError(t *testing.T) {
	t.Parallel()
	err := &v2.RevisionConflictError{Expected: 2, Actual: 3}
	if !errors.Is(err, v2.ErrRevisionConflict) {
		t.Fatalf("errors.Is(%v, ErrRevisionConflict) = false", err)
	}
}

type fakeApplyStore struct{ calls int }

func (s *fakeApplyStore) Apply(context.Context, v2.ApplyRequest) (v2.ApplyResult, error) {
	s.calls++
	return v2.ApplyResult{Applied: true}, nil
}

func validApplyRequest() v2.ApplyRequest {
	soft := int64(8)
	return v2.ApplyRequest{
		Caller: v2.Caller{
			AccountID: "account/test", NamespaceID: "nsp_test", ServiceKeyID: "key_test",
			Operations: []v2.Operation{v2.OperationApply},
		},
		Manifest: v2.Manifest{
			Namespace: v2.Namespace{Account: "account/test", Application: "simorq", Environment: "test"},
			Routes: []v2.RouteSpec{
				{
					Name: "openai", Provider: "openai", Secret: "openai-key",
					AllowedModels: []string{"gpt-4.1"}, DefaultModel: "gpt-4.1", PricingRevision: 1,
					Pricing: []v2.ModelPrice{{Model: "gpt-4.1", InputNanosPerMillionTokens: 1, OutputNanosPerMillionTokens: 1}},
				},
				{
					Name: "local", Provider: "openai", BaseURL: "http://localhost:8080/v1", Secret: "local-key",
					AllowedModels: []string{"local-model"}, DefaultModel: "local-model", PricingRevision: 1,
					Pricing: []v2.ModelPrice{{Model: "local-model", InputNanosPerMillionTokens: 0, OutputNanosPerMillionTokens: 0}},
				},
			},
			Agents: []v2.AgentSpec{
				{Name: "assistant", Kind: v2.AgentLLM, Route: "openai", Enabled: true},
				{Name: "embedder", Kind: v2.AgentEmbedding, Route: "local", Enabled: true},
			},
			Limits: []v2.LimitSpec{
				{Key: "requests", Metric: v2.MetricRequests, Selector: v2.LimitSelector{Agent: "assistant"}, Window: v2.WindowMonth, HardCap: 10, SoftCap: &soft, Enabled: true},
				{Key: "actions", Metric: "ai_actions", Window: v2.WindowDay, HardCap: 5, Enabled: true},
			},
		},
		IdempotencyKey: "deploy/1",
	}
}
