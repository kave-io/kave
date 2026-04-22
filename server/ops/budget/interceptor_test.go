package budget

import (
	"context"
	"errors"
	"testing"
	"time"

	controlmodel "github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/store/sqlite"
	"github.com/kave-io/kave/server/ops/cost"
)

func TestInterceptorBlocksWhenSpendMeetsCap(t *testing.T) {
	app, prices := newBudgetStore(t)
	seedBudgetData(t, app, money.MustParseDollars("5"))
	if err := app.InsertBudgetEntry(context.Background(), &runtimemodel.BudgetEntry{
		ID:           "entry-1",
		ProjectID:    "proj-1",
		EnvID:        "env-1",
		AgentID:      "agent-1",
		RunID:        "run-1",
		Connector:    "openai",
		Cost:         money.MustParseDollars("5"),
		PriceVersion: "test",
		CreatedAt:    time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("seed spend: %v", err)
	}

	ic := New(app, prices)
	action := &runtime.Action{
		Invocation: runtime.Invocation{
			InvocationRef: runtime.InvocationRef{
				ID:        "act-1",
				AgentID:   "agent-1",
				ProjectID: "proj-1",
				EnvID:     "env-1",
				RunID:     "run-1",
			},
			InvocationTarget: runtime.InvocationTarget{
				Type:      runtime.TypeLLM,
				Connector: "openai",
				Method:    "chat.completions",
			},
		},
	}

	_, err := ic.Before(context.Background(), action)
	if err == nil || !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("expected budget block, got %v", err)
	}
	if action.Status != runtime.StatusBlocked {
		t.Fatalf("expected blocked action, got %q", action.Status)
	}
}

func TestInterceptorRecordsUsage(t *testing.T) {
	app, prices := newBudgetStore(t)
	seedBudgetData(t, app, money.MustParseDollars("100"))

	ic := New(app, prices)
	action := &runtime.Action{
		Invocation: runtime.Invocation{
			InvocationRef: runtime.InvocationRef{
				ID:        "act-1",
				AgentID:   "agent-1",
				ProjectID: "proj-1",
				EnvID:     "env-1",
				RunID:     "run-1",
			},
			InvocationTarget: runtime.InvocationTarget{
				Type:      runtime.TypeLLM,
				Connector: "openai",
				Method:    "chat.completions",
			},
		},
	}
	result := &pipeline.Result{
		TokenUsage: &runtime.TokenUsage{
			InputTokens:  10,
			OutputTokens: 20,
			Model:        "gpt-4o",
		},
	}

	if err := ic.After(context.Background(), action, result); err != nil {
		t.Fatalf("after: %v", err)
	}

	spent, err := app.SumAgentSpend(context.Background(), "agent-1", time.Now().Add(-24*time.Hour).UnixMilli())
	if err != nil {
		t.Fatalf("sum spend: %v", err)
	}
	if spent <= 0 {
		t.Fatalf("expected positive spend, got %v", spent)
	}
}

func newBudgetStore(t *testing.T) (store.AppStore, *cost.Service) {
	t.Helper()
	app, err := sqlite.New(t.TempDir() + "/budget-test.db")
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	if err := app.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	prices, err := cost.NewService(context.Background(), app)
	if err != nil {
		t.Fatalf("create cost service: %v", err)
	}
	return app, prices
}

func seedBudgetData(t *testing.T, app store.AppStore, budget money.Amount) {
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
	if err := app.CreateAgent(context.Background(), &controlmodel.Agent{
		ID:            "agent-1",
		ProjectID:     "proj-1",
		EnvID:         "env-1",
		Name:          "agent",
		MonthlyBudget: &budget,
		Status:        controlmodel.AgentStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := app.CreateRun(context.Background(), &runtimemodel.RunRecord{
		ID:        "run-1",
		ProjectID: "proj-1",
		EnvID:     "env-1",
		AgentID:   "agent-1",
		Name:      "run",
		Status:    string(runtime.RunActive),
		Metadata:  map[string]any{},
		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := app.CreateAction(context.Background(), &runtimemodel.ActionRecord{
		ID:         "act-1",
		RunID:      "run-1",
		AgentID:    "agent-1",
		ProjectID:  "proj-1",
		EnvID:      "env-1",
		ActionType: string(runtime.TypeLLM),
		Connector:  "openai",
		Method:     "chat.completions",
		Status:     string(runtime.StatusRunning),
		Source:     string(runtime.ActionSourceIntercepted),
		Metadata:   map[string]any{},
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("create action: %v", err)
	}
}
