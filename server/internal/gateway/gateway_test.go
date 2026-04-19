package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	controlmodel "github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
)

// mockAppStore implements store.AppStore with minimal logic for gateway tests.
type mockAppStore struct {
	agent *controlmodel.Agent
	cred  *controlmodel.ConnectorCredential
}

// AgentStore
func (m *mockAppStore) CreateAgent(_ context.Context, _ *controlmodel.Agent) error { return nil }
func (m *mockAppStore) GetAgentByID(_ context.Context, _ string) (*controlmodel.Agent, error) {
	return m.agent, nil
}
func (m *mockAppStore) GetAgentByName(_ context.Context, _, _ string) (*controlmodel.Agent, error) {
	return nil, nil
}
func (m *mockAppStore) UpdateAgent(_ context.Context, _ string, _ *controlmodel.AgentUpdate) error {
	return nil
}
func (m *mockAppStore) ListAgents(_ context.Context, _ string, _ store.Page) (store.PageResult[*controlmodel.Agent], error) {
	return store.PageResult[*controlmodel.Agent]{}, nil
}
func (m *mockAppStore) DeleteAgent(_ context.Context, _, _ string) error  { return nil }
func (m *mockAppStore) RestoreAgent(_ context.Context, _, _ string) error { return nil }

// PolicyStore
func (m *mockAppStore) CreatePolicy(_ context.Context, _ *controlmodel.PolicyRecord) error {
	return nil
}
func (m *mockAppStore) GetPolicy(_ context.Context, _ string) (*controlmodel.PolicyRecord, error) {
	return nil, nil
}
func (m *mockAppStore) GetAgentPolicy(_ context.Context, _ string) (*controlmodel.PolicyRecord, error) {
	return nil, nil
}
func (m *mockAppStore) ListPolicies(_ context.Context, _ string, _ store.Page) (store.PageResult[*controlmodel.PolicyRecord], error) {
	return store.PageResult[*controlmodel.PolicyRecord]{}, nil
}
func (m *mockAppStore) UpdatePolicy(_ context.Context, _ string, _ *controlmodel.PolicyUpdate) error {
	return nil
}
func (m *mockAppStore) DeletePolicy(_ context.Context, _ string) error { return nil }

// BudgetStore
func (m *mockAppStore) CreateBudget(_ context.Context, _ *controlmodel.Budget) error { return nil }
func (m *mockAppStore) GetBudget(_ context.Context, _ string) (*controlmodel.Budget, error) {
	return nil, nil
}
func (m *mockAppStore) DeleteBudget(_ context.Context, _ string) error { return nil }

// RunStore
func (m *mockAppStore) CreateRun(_ context.Context, _ *runtimemodel.RunRecord) error { return nil }
func (m *mockAppStore) GetRunByID(_ context.Context, _ string) (*runtimemodel.RunRecord, error) {
	return nil, nil
}
func (m *mockAppStore) GetRunByIdempotencyKey(_ context.Context, _, _ string) (*runtimemodel.RunRecord, error) {
	return nil, nil
}
func (m *mockAppStore) UpdateRun(_ context.Context, _ string, _ *runtimemodel.RunUpdate) error {
	return nil
}
func (m *mockAppStore) ListRuns(_ context.Context, _ *runtimemodel.RunFilter, _ store.Page) (store.PageResult[*runtimemodel.RunRecord], error) {
	return store.PageResult[*runtimemodel.RunRecord]{}, nil
}

// ActionStore
func (m *mockAppStore) CreateAction(_ context.Context, _ *runtimemodel.ActionRecord) error {
	return nil
}
func (m *mockAppStore) GetAction(_ context.Context, _ string) (*runtimemodel.ActionRecord, error) {
	return nil, nil
}
func (m *mockAppStore) ListActionsByRun(_ context.Context, _ string, _ store.Page) (store.PageResult[*runtimemodel.ActionRecord], error) {
	return store.PageResult[*runtimemodel.ActionRecord]{}, nil
}

// CostStore
func (m *mockAppStore) GetPriceBook(_ context.Context) (*runtimemodel.PriceBook, error) {
	return nil, nil
}
func (m *mockAppStore) SavePriceBook(_ context.Context, _ *runtimemodel.PriceBook) error { return nil }
func (m *mockAppStore) ListFXRates(_ context.Context) ([]runtimemodel.FXRateRecord, error) {
	return nil, nil
}
func (m *mockAppStore) GetFXRate(_ context.Context, _, _ money.CurrencyCode) (*runtimemodel.FXRateRecord, error) {
	return nil, nil
}
func (m *mockAppStore) UpsertFXRates(_ context.Context, _ []runtimemodel.FXRateRecord) error {
	return nil
}
func (m *mockAppStore) ListFXCurrencies(_ context.Context) ([]runtimemodel.FXCurrencyRecord, error) {
	return nil, nil
}
func (m *mockAppStore) UpsertFXCurrencies(_ context.Context, _ []runtimemodel.FXCurrencyRecord) error {
	return nil
}
func (m *mockAppStore) InsertBudgetEntry(_ context.Context, _ *runtimemodel.BudgetEntry) error {
	return nil
}
func (m *mockAppStore) AddRunSpend(_ context.Context, _ string, _ money.Amount) error { return nil }
func (m *mockAppStore) SumAgentSpend(_ context.Context, _ string, _ int64) (money.Amount, error) {
	return 0, nil
}
func (m *mockAppStore) GetSpendReport(_ context.Context, _ *runtimemodel.SpendFilter) (*runtimemodel.SpendReport, error) {
	return &runtimemodel.SpendReport{}, nil
}

// TokenStore
func (m *mockAppStore) InsertAgentToken(_ context.Context, _ *controlmodel.AgentToken) error {
	return nil
}
func (m *mockAppStore) GetTokenByHash(_ context.Context, _ string) (*controlmodel.AgentToken, error) {
	return nil, nil
}
func (m *mockAppStore) GetToken(_ context.Context, _ string) (*controlmodel.AgentToken, error) {
	return nil, nil
}
func (m *mockAppStore) ListTokens(_ context.Context, _ string, _ store.Page) (store.PageResult[*controlmodel.AgentToken], error) {
	return store.PageResult[*controlmodel.AgentToken]{}, nil
}
func (m *mockAppStore) RevokeToken(_ context.Context, _, _, _ string) error { return nil }
func (m *mockAppStore) TouchToken(_ context.Context, _ string) error        { return nil }

// CredentialStore
func (m *mockAppStore) GetCredential(_ context.Context, _ string) (*controlmodel.ConnectorCredential, error) {
	return m.cred, nil
}
func (m *mockAppStore) StoreCredential(_ context.Context, _ *controlmodel.ConnectorCredential) error {
	return nil
}
func (m *mockAppStore) DeleteCredential(_ context.Context, _ string) error { return nil }
func (m *mockAppStore) ListCredentials(_ context.Context, _ string, _ store.Page) (store.PageResult[*controlmodel.ConnectorCredential], error) {
	return store.PageResult[*controlmodel.ConnectorCredential]{}, nil
}
func (m *mockAppStore) ResolveCredential(_ context.Context, _ *controlmodel.CredentialFilter) (*controlmodel.ConnectorCredential, error) {
	return m.cred, nil
}
func (m *mockAppStore) RotateCredential(_ context.Context, _ string, _ []byte, _, _ string) error {
	return nil
}
func (m *mockAppStore) RevokeCredential(_ context.Context, _, _, _ string) error { return nil }
func (m *mockAppStore) TouchCredential(_ context.Context, _ string) error        { return nil }

// OrgStore
func (m *mockAppStore) CreateOrg(_ context.Context, _ *controlmodel.Organization) error { return nil }
func (m *mockAppStore) GetOrg(_ context.Context, _ string) (*controlmodel.Organization, error) {
	return nil, nil
}
func (m *mockAppStore) GetOrgBySlug(_ context.Context, _ string) (*controlmodel.Organization, error) {
	return nil, nil
}
func (m *mockAppStore) ListOrgs(_ context.Context, _ store.Page) (store.PageResult[*controlmodel.Organization], error) {
	return store.PageResult[*controlmodel.Organization]{}, nil
}

// UserStore
func (m *mockAppStore) CreateUser(_ context.Context, _ *controlmodel.User) error { return nil }
func (m *mockAppStore) GetUser(_ context.Context, _ string) (*controlmodel.User, error) {
	return nil, nil
}
func (m *mockAppStore) GetUserByEmail(_ context.Context, _, _ string) (*controlmodel.User, error) {
	return nil, nil
}
func (m *mockAppStore) UpdateUser(_ context.Context, _ string, _ *controlmodel.UserUpdate) error {
	return nil
}

// MembershipStore
func (m *mockAppStore) AddMember(_ context.Context, _ *controlmodel.Membership) error { return nil }
func (m *mockAppStore) GetMembership(_ context.Context, _, _ string) (*controlmodel.Membership, error) {
	return nil, nil
}
func (m *mockAppStore) ListMembers(_ context.Context, _ string, _ store.Page) (store.PageResult[*controlmodel.Membership], error) {
	return store.PageResult[*controlmodel.Membership]{}, nil
}
func (m *mockAppStore) RemoveMember(_ context.Context, _, _ string) error { return nil }

// ProjectStore
func (m *mockAppStore) CreateProject(_ context.Context, _ *controlmodel.Project) error { return nil }
func (m *mockAppStore) GetProject(_ context.Context, _ string) (*controlmodel.Project, error) {
	return nil, nil
}
func (m *mockAppStore) ListProjects(_ context.Context, _ string, _ store.Page) (store.PageResult[*controlmodel.Project], error) {
	return store.PageResult[*controlmodel.Project]{}, nil
}

// EnvironmentStore
func (m *mockAppStore) CreateEnvironment(_ context.Context, _ *controlmodel.Environment) error {
	return nil
}
func (m *mockAppStore) GetEnvironment(_ context.Context, _ string) (*controlmodel.Environment, error) {
	return nil, nil
}
func (m *mockAppStore) GetEnvironmentBySlug(_ context.Context, _, _ string) (*controlmodel.Environment, error) {
	return nil, nil
}
func (m *mockAppStore) ListEnvironments(_ context.Context, _ string, _ store.Page) (store.PageResult[*controlmodel.Environment], error) {
	return store.PageResult[*controlmodel.Environment]{}, nil
}

// StoreLifecycle
func (m *mockAppStore) WithTx(_ context.Context, fn func(store.AppStore) error) error { return fn(m) }
func (m *mockAppStore) Migrate(_ context.Context) error                               { return nil }
func (m *mockAppStore) Close() error                                                  { return nil }

var _ store.AppStore = (*mockAppStore)(nil)

func TestGatewayAgentNotFound(t *testing.T) {
	g := New(&mockAppStore{}, nil, pipeline.New())
	mux := http.NewServeMux()
	g.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/frameworks/claude-code/openai/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d want 401", w.Code)
	}
}

func TestGatewayForwardsClaudeCodeOpenAI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer real-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("got path %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "gpt-4o",
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 20,
			},
		})
	}))
	defer upstream.Close()

	g := New(&mockAppStore{
		agent: &controlmodel.Agent{ID: "a1", ProjectID: "proj1", EnvID: "default", Status: controlmodel.AgentStatusActive},
		cred:  &controlmodel.ConnectorCredential{EncryptedBlob: []byte("real-key"), ProjectID: "proj1"},
	}, nil, pipeline.New())
	g.transport.client.Transport = rewriteTransport(t, upstream.URL)

	mux := http.NewServeMux()
	g.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]any{
		"model":    "gpt-4o",
		"messages": []any{},
	})
	req := httptest.NewRequest(http.MethodPost, "/frameworks/claude-code/openai/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer a1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d want 200 body=%s", w.Code, w.Body.String())
	}
}

func rewriteTransport(t *testing.T, upstream string) http.RoundTripper {
	t.Helper()
	target, err := url.Parse(upstream)
	if err != nil {
		t.Fatalf("parse upstream: %v", err)
	}

	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		return http.DefaultTransport.RoundTrip(req)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
