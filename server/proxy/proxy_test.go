package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kave-io/kave/core/intercept"
	"github.com/kave-io/kave/core/store"
)

// ── minimal store mocks ───────────────────────────────────────────────────────

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

// satisfy full interface with no-ops
func (m *mockAppStore) CreateWorkspace(_ context.Context, _ *store.Workspace) error  { return nil }
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
func (m *mockAppStore) InsertAgentToken(_ context.Context, _ *store.AgentToken) error { return nil }
func (m *mockAppStore) IsTokenRevoked(_ context.Context, _ string) (bool, error)       { return false, nil }
func (m *mockAppStore) InsertRevokedToken(_ context.Context, _ string) error           { return nil }
func (m *mockAppStore) StoreCredential(_ context.Context, _ *store.Credential) error   { return nil }
func (m *mockAppStore) DeleteCredential(_ context.Context, _ string) error             { return nil }
func (m *mockAppStore) WithTx(_ context.Context, fn func(store.AppStore) error) error  { return fn(m) }
func (m *mockAppStore) Migrate(_ context.Context) error                                { return nil }
func (m *mockAppStore) Close() error                                                   { return nil }

// ── tests ─────────────────────────────────────────────────────────────────────

func TestProxyMissingAuth(t *testing.T) {
	p := New(&mockAppStore{}, nil, intercept.New())
	mux := http.NewServeMux()
	p.RegisterRoutes(mux)

	r := httptest.NewRequest(http.MethodPost, "/proxy/openai/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("got %d want 401", w.Code)
	}
}

func TestProxyAgentNotFound(t *testing.T) {
	app := &mockAppStore{agent: nil} // no agent
	p := New(app, nil, intercept.New())
	mux := http.NewServeMux()
	p.RegisterRoutes(mux)

	r := httptest.NewRequest(http.MethodPost, "/proxy/openai/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer unknown-agent")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("got %d want 401", w.Code)
	}
}

func TestProxyNoCredential(t *testing.T) {
	app := &mockAppStore{
		agent: &store.Agent{ID: "a1", WorkspaceID: "ws1"},
		cred:  nil, // no credential
	}
	p := New(app, nil, intercept.New())
	mux := http.NewServeMux()
	p.RegisterRoutes(mux)

	r := httptest.NewRequest(http.MethodPost, "/proxy/openai/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer a1")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("got %d want 403", w.Code)
	}
}

func TestProxyForwardsToUpstream(t *testing.T) {
	// Mock upstream that returns a valid OpenAI-shaped response
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header was injected
		if r.Header.Get("Authorization") != "Bearer real-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"model": "gpt-4o",
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 20,
			},
		})
	}))
	defer upstream.Close()

	// Point the upstream client at our mock server
	app := &mockAppStore{
		agent: &store.Agent{ID: "a1", WorkspaceID: "ws1"},
		cred:  &store.Credential{Encrypted: []byte("real-key")}, // plaintext (no enc key)
	}

	p := New(app, nil, intercept.New())
	// Override base URL to point at mock upstream
	p.upstream.client.Transport = nil // use default
	origURLs := baseURLs["openai"]
	baseURLs["openai"] = upstream.URL
	defer func() { baseURLs["openai"] = origURLs }()

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]any{"model": "gpt-4o", "messages": []any{}})
	r := httptest.NewRequest(http.MethodPost, "/proxy/openai/v1/chat/completions", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer a1")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("got %d want 200 — body: %s", w.Code, w.Body.String())
	}
}

// ── token extraction ──────────────────────────────────────────────────────────

func TestExtractTokenUsage(t *testing.T) {
	tests := []struct {
		connector    string
		body         string
		wantInput    int
		wantOutput   int
		wantModel    string
	}{
		{
			connector: "openai",
			body:      `{"model":"gpt-4o","usage":{"prompt_tokens":100,"completion_tokens":50}}`,
			wantInput: 100, wantOutput: 50, wantModel: "gpt-4o",
		},
		{
			connector: "anthropic",
			body:      `{"model":"claude-sonnet-4-5","usage":{"input_tokens":80,"output_tokens":40}}`,
			wantInput: 80, wantOutput: 40, wantModel: "claude-sonnet-4-5",
		},
		{
			connector: "ollama",
			body:      `{"model":"mistral","prompt_eval_count":60,"eval_count":30}`,
			wantInput: 60, wantOutput: 30, wantModel: "mistral",
		},
		{
			connector: "groq",
			body:      `{"model":"llama3","usage":{"prompt_tokens":20,"completion_tokens":10}}`,
			wantInput: 20, wantOutput: 10, wantModel: "llama3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.connector, func(t *testing.T) {
			usage := ExtractTokenUsage([]byte(tt.body), tt.connector)
			if usage == nil {
				t.Fatal("got nil usage")
			}
			if got := usage["InputTokens"].(int); got != tt.wantInput {
				t.Errorf("InputTokens: got %d want %d", got, tt.wantInput)
			}
			if got := usage["OutputTokens"].(int); got != tt.wantOutput {
				t.Errorf("OutputTokens: got %d want %d", got, tt.wantOutput)
			}
			if got, _ := usage["Model"].(string); got != tt.wantModel {
				t.Errorf("Model: got %q want %q", got, tt.wantModel)
			}
		})
	}
}

// keep time import happy
var _ = time.Now
