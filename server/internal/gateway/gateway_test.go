package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/kave-io/kave/core/intercept"
	"github.com/kave-io/kave/core/store"
)

type mockAppStore struct {
	agent *store.Agent
	cred  *store.Credential
}

func (m *mockAppStore) GetAgentByID(_ context.Context, _ string) (*store.Agent, error) {
	return m.agent, nil
}
func (m *mockAppStore) GetCredential(_ context.Context, _, _ string) (*store.Credential, error) {
	return m.cred, nil
}
func (m *mockAppStore) CreateWorkspace(_ context.Context, _ *store.Workspace) error { return nil }
func (m *mockAppStore) GetWorkspace(_ context.Context, _ string) (*store.Workspace, error) {
	return nil, nil
}
func (m *mockAppStore) CreateAgent(_ context.Context, _ *store.Agent) error { return nil }
func (m *mockAppStore) GetAgentByName(_ context.Context, _, _ string) (*store.Agent, error) {
	return nil, nil
}
func (m *mockAppStore) UpdateAgent(_ context.Context, _ string, _ *store.AgentUpdate) error {
	return nil
}
func (m *mockAppStore) ListAgents(_ context.Context, _ string) ([]*store.Agent, error) {
	return nil, nil
}
func (m *mockAppStore) CreatePolicy(_ context.Context, _ *store.Policy) error { return nil }
func (m *mockAppStore) GetPolicy(_ context.Context, _ string) (*store.Policy, error) {
	return nil, nil
}
func (m *mockAppStore) GetAgentPolicy(_ context.Context, _ string) (*store.Policy, error) {
	return nil, nil
}
func (m *mockAppStore) CreateRun(_ context.Context, _ *store.Run) error { return nil }
func (m *mockAppStore) CreateAction(_ context.Context, _ *store.ActionRecord) error {
	return nil
}
func (m *mockAppStore) GetRunByID(_ context.Context, _ string) (*store.Run, error) {
	return nil, nil
}
func (m *mockAppStore) UpdateRun(_ context.Context, _ string, _ *store.RunUpdate) error { return nil }
func (m *mockAppStore) ListRuns(_ context.Context, _ *store.RunFilter) ([]*store.Run, error) {
	return nil, nil
}
func (m *mockAppStore) InsertBudgetEntry(_ context.Context, _ *store.BudgetEntry) error { return nil }
func (m *mockAppStore) AddRunSpend(_ context.Context, _ string, _ float64) error        { return nil }
func (m *mockAppStore) SumAgentSpend(_ context.Context, _ string, _ int64) (float64, error) {
	return 0, nil
}
func (m *mockAppStore) GetSpendReport(_ context.Context, _ *store.SpendFilter) (*store.SpendReport, error) {
	return &store.SpendReport{}, nil
}
func (m *mockAppStore) GetPriceBook(_ context.Context) (*store.PriceBook, error)      { return nil, nil }
func (m *mockAppStore) SavePriceBook(_ context.Context, _ *store.PriceBook) error     { return nil }
func (m *mockAppStore) InsertAgentToken(_ context.Context, _ *store.AgentToken) error { return nil }
func (m *mockAppStore) IsTokenRevoked(_ context.Context, _ string) (bool, error)      { return false, nil }
func (m *mockAppStore) InsertRevokedToken(_ context.Context, _ string) error          { return nil }
func (m *mockAppStore) StoreCredential(_ context.Context, _ *store.Credential) error  { return nil }
func (m *mockAppStore) DeleteCredential(_ context.Context, _ string) error            { return nil }
func (m *mockAppStore) WithTx(_ context.Context, fn func(store.AppStore) error) error { return fn(m) }
func (m *mockAppStore) Migrate(_ context.Context) error                               { return nil }
func (m *mockAppStore) Close() error                                                  { return nil }

func TestGatewayAgentNotFound(t *testing.T) {
	g := New(&mockAppStore{}, nil, intercept.New())
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
		agent: &store.Agent{ID: "a1", WorkspaceID: "ws1"},
		cred:  &store.Credential{Encrypted: []byte("real-key")},
	}, nil, intercept.New())
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
