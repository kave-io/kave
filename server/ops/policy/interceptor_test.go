package policy

import (
	"context"
	"errors"
	"testing"
	"time"

	controlmodel "github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/store/sqlite"
)

func TestInterceptorBlocksDisallowedConnector(t *testing.T) {
	app := newSQLiteAppStore(t)
	seedPolicyTestData(t, app, &controlmodel.PolicyRecord{
		ID:                "pol-1",
		ProjectID:         "proj-1",
		EnvID:             "env-1",
		Name:              "policy",
		AllowedTypes:      []string{"llm"},
		AllowedConnectors: []string{"anthropic"},
		AllowedMethods:    []string{"messages"},
		Status:            string(controlmodel.PolicyStatusActive),
		CreatedAt:         time.Now().UnixMilli(),
		UpdatedAt:         time.Now().UnixMilli(),
	})

	ic := New(app, nil)
	action := &runtime.Action{
		Invocation: runtime.Invocation{
			InvocationRef: runtime.InvocationRef{
				ID:        "act-1",
				AgentID:   "agent-1",
				ProjectID: "proj-1",
				EnvID:     "env-1",
			},
			InvocationTarget: runtime.InvocationTarget{
				Type:      runtime.TypeLLM,
				Connector: "openai",
				Method:    "chat.completions",
			},
		},
	}

	_, err := ic.Before(context.Background(), action)
	if err == nil || !errors.Is(err, ErrPolicyBlocked) {
		t.Fatalf("expected policy block, got %v", err)
	}
	if action.Status != runtime.StatusBlocked {
		t.Fatalf("expected blocked action, got %q", action.Status)
	}
}

func TestInterceptorAllowsMatchingAction(t *testing.T) {
	app := newSQLiteAppStore(t)
	seedPolicyTestData(t, app, &controlmodel.PolicyRecord{
		ID:                "pol-1",
		ProjectID:         "proj-1",
		EnvID:             "env-1",
		Name:              "policy",
		AllowedTypes:      []string{"llm"},
		AllowedConnectors: []string{"openai"},
		AllowedMethods:    []string{"chat.completions"},
		Status:            string(controlmodel.PolicyStatusActive),
		CreatedAt:         time.Now().UnixMilli(),
		UpdatedAt:         time.Now().UnixMilli(),
	})

	ic := New(app, nil)
	action := &runtime.Action{
		Invocation: runtime.Invocation{
			InvocationRef: runtime.InvocationRef{
				ID:        "act-1",
				AgentID:   "agent-1",
				ProjectID: "proj-1",
				EnvID:     "env-1",
			},
			InvocationTarget: runtime.InvocationTarget{
				Type:      runtime.TypeLLM,
				Connector: "openai",
				Method:    "chat.completions",
			},
		},
	}

	got, err := ic.Before(context.Background(), action)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != action {
		t.Fatalf("expected action passthrough")
	}
}

func newSQLiteAppStore(t *testing.T) store.AppStore {
	t.Helper()
	app, err := sqlite.New(t.TempDir() + "/policy-test.db")
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	if err := app.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	return app
}

func seedPolicyTestData(t *testing.T, app store.AppStore, pol *controlmodel.PolicyRecord) {
	t.Helper()
	now := time.Now().UnixMilli()
	if err := app.CreateOrg(context.Background(), &controlmodel.Organization{ID: "org-1", Name: "Org", Slug: "org-1", Plan: "free", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := app.CreateProject(context.Background(), &controlmodel.Project{ID: "proj-1", OrgID: "org-1", Name: "Project", Slug: "proj-1", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := app.CreateEnvironment(context.Background(), &controlmodel.Environment{ID: "env-1", ProjectID: "proj-1", Name: "Env", Slug: "env-1", Type: "dev", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create env: %v", err)
	}
	if err := app.CreatePolicy(context.Background(), pol); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	polID := pol.ID
	if err := app.CreateAgent(context.Background(), &controlmodel.Agent{
		ID:        "agent-1",
		ProjectID: "proj-1",
		EnvID:     "env-1",
		Name:      "agent",
		PolicyID:  &polID,
		Status:    controlmodel.AgentStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
}
