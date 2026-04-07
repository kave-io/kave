package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kave-io/kave/core/store"
)

// ── agents ────────────────────────────────────────────────────────────────────

func TestCreateAgent(t *testing.T) {
	a := New(&mockAppStore{}, &mockSpanStore{}, nil)

	tests := []struct {
		name   string
		body   any
		status int
	}{
		{"valid", createAgentReq{WorkspaceID: "ws1", Name: "bot"}, http.StatusCreated},
		{"missing name", createAgentReq{WorkspaceID: "ws1"}, http.StatusBadRequest},
		{"missing workspace", createAgentReq{Name: "bot"}, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			r := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(body))
			w := httptest.NewRecorder()
			a.createAgent(w, r)
			if w.Code != tt.status {
				t.Errorf("got %d want %d", w.Code, tt.status)
			}
		})
	}
}

func TestGetAgent(t *testing.T) {
	agent := &store.Agent{ID: "a1", WorkspaceID: "ws1", Name: "bot"}
	app := &mockAppStore{}
	app.getAgentByID = func(_ context.Context, id string) (*store.Agent, error) {
		if id == "a1" {
			return agent, nil
		}
		return nil, nil
	}
	a := New(app, &mockSpanStore{}, nil)
	mux := http.NewServeMux()
	a.RegisterRoutes(mux)

	t.Run("found", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/agents/a1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("got %d want 200", w.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/agents/missing", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("got %d want 404", w.Code)
		}
	})
}

func TestListAgents(t *testing.T) {
	app := &mockAppStore{}
	app.listAgents = func(_ context.Context, wsID string) ([]*store.Agent, error) {
		return []*store.Agent{{ID: "a1"}}, nil
	}
	a := New(app, &mockSpanStore{}, nil)

	t.Run("missing workspace_id", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
		w := httptest.NewRecorder()
		a.listAgents(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("got %d want 400", w.Code)
		}
	})

	t.Run("with workspace_id", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/agents?workspace_id=ws1", nil)
		w := httptest.NewRecorder()
		a.listAgents(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("got %d want 200", w.Code)
		}
	})
}

// ── policies ──────────────────────────────────────────────────────────────────

func TestCreatePolicy(t *testing.T) {
	a := New(&mockAppStore{}, &mockSpanStore{}, nil)

	tests := []struct {
		name   string
		body   any
		status int
	}{
		{"valid", createPolicyReq{WorkspaceID: "ws1", Name: "p1"}, http.StatusCreated},
		{"missing name", createPolicyReq{WorkspaceID: "ws1"}, http.StatusBadRequest},
		{"missing workspace", createPolicyReq{Name: "p1"}, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			r := httptest.NewRequest(http.MethodPost, "/api/v1/policies", bytes.NewReader(body))
			w := httptest.NewRecorder()
			a.createPolicy(w, r)
			if w.Code != tt.status {
				t.Errorf("got %d want %d", w.Code, tt.status)
			}
		})
	}
}

func TestGetPolicy(t *testing.T) {
	app := &mockAppStore{}
	app.getPolicy = func(_ context.Context, id string) (*store.Policy, error) {
		if id == "p1" {
			return &store.Policy{ID: "p1"}, nil
		}
		return nil, nil
	}
	a := New(app, &mockSpanStore{}, nil)
	mux := http.NewServeMux()
	a.RegisterRoutes(mux)

	t.Run("found", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/policies/p1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("got %d want 200", w.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/policies/missing", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("got %d want 404", w.Code)
		}
	})
}

// ── runs ──────────────────────────────────────────────────────────────────────

func TestListRuns(t *testing.T) {
	a := New(&mockAppStore{}, &mockSpanStore{}, nil)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/runs?workspace_id=ws1&status=running&limit=5", nil)
	w := httptest.NewRecorder()
	a.listRuns(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d want 200", w.Code)
	}
}

func TestGetRun(t *testing.T) {
	run := &store.Run{ID: "r1", WorkspaceID: "ws1"}
	app := &mockAppStore{}
	app.getRunByID = func(_ context.Context, id string) (*store.Run, error) {
		if id == "r1" {
			return run, nil
		}
		return nil, nil
	}
	a := New(app, &mockSpanStore{}, nil)
	mux := http.NewServeMux()
	a.RegisterRoutes(mux)

	t.Run("found", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/runs/r1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("got %d want 200", w.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/runs/missing", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("got %d want 404", w.Code)
		}
	})
}

func TestGetRunSpans(t *testing.T) {
	spans := &mockSpanStore{}
	spans.querySpans = func(_ context.Context, f *store.SpanFilter) ([]*store.SpanRow, error) {
		return []*store.SpanRow{{ID: "s1", RunID: f.RunID}}, nil
	}
	a := New(&mockAppStore{}, spans, nil)
	mux := http.NewServeMux()
	a.RegisterRoutes(mux)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/runs/r1/spans?limit=5", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d want 200", w.Code)
	}
}

// ── spans ─────────────────────────────────────────────────────────────────────

func TestListSpans(t *testing.T) {
	spans := &mockSpanStore{}
	spans.querySpans = func(_ context.Context, _ *store.SpanFilter) ([]*store.SpanRow, error) {
		return []*store.SpanRow{{ID: "s1"}}, nil
	}
	a := New(&mockAppStore{}, spans, nil)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/spans?run_id=r1&limit=10&has_error=false", nil)
	w := httptest.NewRecorder()
	a.listSpans(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d want 200", w.Code)
	}
}

func TestListSpans_StoreError(t *testing.T) {
	spans := &mockSpanStore{}
	spans.querySpans = func(_ context.Context, _ *store.SpanFilter) ([]*store.SpanRow, error) {
		return nil, errors.New("db error")
	}
	a := New(&mockAppStore{}, spans, nil)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/spans", nil)
	w := httptest.NewRecorder()
	a.listSpans(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d want 500", w.Code)
	}
}

// ── cost ──────────────────────────────────────────────────────────────────────

func TestCostSummary(t *testing.T) {
	a := New(&mockAppStore{}, &mockSpanStore{}, nil)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/cost/summary?agent_id=a1", nil)
	w := httptest.NewRecorder()
	a.costSummary(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("got %d want 200", w.Code)
	}
}

func TestCostSummary_StoreError(t *testing.T) {
	app := &mockAppStore{}
	app.getSpendReport = func(_ context.Context, _ *store.SpendFilter) (*store.SpendReport, error) {
		return nil, errors.New("db error")
	}
	a := New(app, &mockSpanStore{}, nil)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/cost/summary", nil)
	w := httptest.NewRecorder()
	a.costSummary(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("got %d want 500", w.Code)
	}
}
