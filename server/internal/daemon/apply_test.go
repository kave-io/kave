package daemon

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kave-io/kave/core/bus"
	controlmodel "github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/server/internal/config"
	sqliteStore "github.com/kave-io/kave/server/internal/store/sqlite"
	"github.com/kave-io/kave/server/ops/auth/credresolve"
)

func TestBuildPlanAndApply(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "provider-secret")
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
			{
				Name:        "permissive",
				Description: "permissive policy",
				Mode:        "enforce",
				Auth: map[string]any{
					"allowedTypes":      []any{"*"},
					"allowedConnectors": []any{"*"},
					"allowedMethods":    []any{"*"},
				},
				Trace: map[string]any{
					"input":         false,
					"output":        false,
					"retentionDays": 1,
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
				Name:        "strict-bot",
				Description: "strict agent",
				Env:         "dev",
				Policy:      "strict",
				Credentials: []string{"openai-primary"},
				Status:      "active",
			},
			{
				Name:        "permissive-bot",
				Description: "permissive agent",
				Env:         "dev",
				Policy:      "permissive",
				Credentials: []string{"openai-primary"},
				Status:      "active",
			},
			{
				Name:        "readonly-bot",
				Description: "readonly agent",
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

	stored, err := app.GetCredential(context.Background(), "openai-primary")
	if err != nil {
		t.Fatalf("GetCredential() error = %v", err)
	}
	if stored == nil {
		t.Fatal("GetCredential() returned nil")
	}
	if stored.SourceType != controlmodel.CredSourceEnv || stored.Source != controlmodel.CredentialSourceEnv {
		t.Fatalf("stored source = (%q, %q), want env", stored.SourceType, stored.Source)
	}
	if stored.SecretRef != "OPENAI_API_KEY" || stored.EnvVar != "OPENAI_API_KEY" {
		t.Fatalf("stored env reference = (%q, %q)", stored.SecretRef, stored.EnvVar)
	}
	resolved, err := credresolve.Resolve(context.Background(), stored, nil)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved != "provider-secret" {
		t.Fatalf("Resolve() = %q, want provider-secret", resolved)
	}

	nextPlan, err := state.BuildPlan(context.Background())
	if err != nil {
		t.Fatalf("BuildPlan() after apply error = %v", err)
	}
	if got := len(nextPlan.Creates) + len(nextPlan.Updates) + len(nextPlan.Deletes); got != 0 {
		t.Fatalf("expected empty plan after apply, got %#v", nextPlan)
	}
}

func TestDesiredCredentialSourceCanonicalAndUnsupportedSources(t *testing.T) {
	tests := []struct {
		name       string
		config     config.CredentialConfig
		wantSource string
		wantRef    string
		wantErr    string
	}{
		{name: "env", config: config.CredentialConfig{Source: "env", Env: "PROVIDER_KEY"}, wantSource: controlmodel.CredSourceEnv, wantRef: "PROVIDER_KEY"},
		{name: "legacy vault", config: config.CredentialConfig{Source: "vault", Ref: "kv/provider"}, wantSource: controlmodel.CredSourceVaultRef, wantRef: "kv/provider"},
		{name: "canonical vault", config: config.CredentialConfig{Source: "vault_ref", Ref: "kv/provider"}, wantSource: controlmodel.CredSourceVaultRef, wantRef: "kv/provider"},
		{name: "passthrough", config: config.CredentialConfig{Source: "passthrough"}, wantSource: controlmodel.CredSourcePassthrough},
		{name: "file", config: config.CredentialConfig{Source: "file", File: "/tmp/key"}, wantErr: "source file is not supported"},
		{name: "oauth", config: config.CredentialConfig{Source: "oauth"}, wantErr: "source oauth is not supported"},
		{name: "sts", config: config.CredentialConfig{Source: "sts"}, wantErr: "source sts is not supported"},
		{name: "encrypted", config: config.CredentialConfig{Source: "encrypted"}, wantErr: "cannot be declared in config"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, ref, err := desiredCredentialSource(test.config)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("desiredCredentialSource() error = %v, want containing %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("desiredCredentialSource() error = %v", err)
			}
			if source != test.wantSource || ref != test.wantRef {
				t.Fatalf("desiredCredentialSource() = (%q, %q), want (%q, %q)", source, ref, test.wantSource, test.wantRef)
			}
		})
	}
}

func TestBuildPlanRejectsUnsupportedCredentialBeforePersistence(t *testing.T) {
	app, err := sqliteStore.New(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("sqlite store: %v", err)
	}
	defer app.Close()

	state := New(config.LoadOpts{}, &config.LoadResult{Config: &config.Config{
		Credentials: []config.CredentialConfig{{Name: "provider", Source: "file", File: "/tmp/key"}},
	}}, app, nil, nil, nil, bus.New(), "test")

	_, err = state.BuildPlan(context.Background())
	if err == nil || !strings.Contains(err.Error(), `credential "provider": source file is not supported`) {
		t.Fatalf("BuildPlan() error = %v", err)
	}
}
