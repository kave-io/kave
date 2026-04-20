package daemon

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kave-io/kave/core/bus"
	controlmodel "github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/server/internal/config"
	sqliteStore "github.com/kave-io/kave/server/internal/store/sqlite"
)

func TestBuildPlanAndApply(t *testing.T) {
	dir := t.TempDir()
	app, err := sqliteStore.New(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatalf("sqlite store: %v", err)
	}
	defer app.Close()

	if err := app.CreateOrg(context.Background(), &controlmodel.Organization{
		ID:        "default",
		Name:      "Default",
		Slug:      "default",
		CreatedAt: 1,
		UpdatedAt: 1,
	}); err != nil {
		t.Fatalf("seed org: %v", err)
	}

	cfg := &config.Config{
		Project: &config.ProjectConfig{
			Name:       "demo",
			Slug:       "demo",
			DefaultEnv: "dev",
			Envs: []config.ProjectEnv{
				{Name: "dev", Slug: "dev", Type: "dev"},
			},
		},
		Policies: []config.PolicyConfig{
			{
				Name:        "strict",
				Description: "strict policy",
				Mode:        "enforce",
				Auth: map[string]any{
					"allowedTypes":      []any{"llm"},
					"allowedConnectors": []any{"openai"},
					"allowedMethods":    []any{"chat.completions"},
				},
				Trace: map[string]any{
					"input":         true,
					"output":        true,
					"retentionDays": 7,
				},
			},
		},
		Credentials: []config.CredentialConfig{
			{
				Name:      "openai-primary",
				Connector: "openai",
				Label:     "primary",
				Source:    "env",
				Env:       "OPENAI_API_KEY",
			},
		},
		Agents: []config.AgentConfig{
			{
				Name:        "bot",
				Description: "bot agent",
				Env:         "dev",
				Policy:      "strict",
				Credentials: []string{"openai-primary"},
				Status:      "active",
			},
		},
	}

	state := New(config.LoadOpts{}, &config.LoadResult{
		Config: cfg,
		Origin: map[string]config.Source{"project.name": config.SourceProject},
	}, app, nil, nil, nil, bus.New(), "test")

	plan, err := state.BuildPlan(context.Background())
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if len(plan.Creates) == 0 {
		t.Fatalf("expected create operations")
	}

	report, err := state.Apply(context.Background(), plan, false)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if report.Created == 0 {
		t.Fatalf("expected created resources")
	}

	nextPlan, err := state.BuildPlan(context.Background())
	if err != nil {
		t.Fatalf("BuildPlan() after apply error = %v", err)
	}
	if got := len(nextPlan.Creates) + len(nextPlan.Updates) + len(nextPlan.Deletes); got != 0 {
		t.Fatalf("expected empty plan after apply, got %#v", nextPlan)
	}
}
