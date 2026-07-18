package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadLayeringAndOrigins(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	userDir := filepath.Join(home, ".kave")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user dir: %v", err)
	}

	projectFile := filepath.Join(root, "kave.yaml")
	userFile := filepath.Join(userDir, "kave.yaml")

	if err := os.WriteFile(projectFile, []byte(`
storage:
  defaults:
    app:
      path: project.db
fx:
  refresh_interval_seconds: 10
agents:
  - name: shared
    description: project
    env: dev
    policy: strict
    credentials: [base]
  - name: project-only
    description: project
    env: dev
    policy: strict
    credentials: [base]
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	if err := os.WriteFile(userFile, []byte(`
storage:
  defaults:
    app:
      kind: sqlite
agents:
  - name: shared
    description: user
    env: dev
    policy: exploratory
    credentials: [user]
  - name: user-only
    description: user
    env: dev
    policy: exploratory
    credentials: [user]
`), 0o644); err != nil {
		t.Fatalf("write user: %v", err)
	}

	res, err := Load(LoadOpts{
		StartDir: root,
		Env: map[string]string{
			"KAVE_FX_REFRESH_INTERVAL_SECONDS": "30",
		},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got := res.Config.FX.RefreshIntervalSeconds; got != 30 {
		t.Fatalf("FX.RefreshIntervalSeconds = %d, want 30", got)
	}
	if got := res.Config.Storage.Defaults.App.Kind; got != "sqlite" {
		t.Fatalf("Storage.Defaults.App.Kind = %q, want sqlite", got)
	}
	if got := res.Config.Storage.Defaults.App.Path; got != "project.db" {
		t.Fatalf("Storage.Defaults.App.Path = %q, want project.db", got)
	}
	if res.Config.Security.AllowAnonymous {
		t.Fatal("Security.AllowAnonymous defaulted open; want fail-closed false")
	}

	if got := res.Origin["fx.refresh_interval_seconds"]; got != SourceEnv {
		t.Fatalf("origin fx.refresh_interval_seconds = %q, want %q", got, SourceEnv)
	}
	if got := res.Origin["storage.defaults.app.kind"]; got != SourceUser {
		t.Fatalf("origin storage.defaults.app.kind = %q, want %q", got, SourceUser)
	}
	if got := res.Origin["storage.defaults.app.path"]; got != SourceProject {
		t.Fatalf("origin storage.defaults.app.path = %q, want %q", got, SourceProject)
	}
	if got := res.Origin["agents.shared.description"]; got != SourceProject {
		t.Fatalf("origin agents.shared.description = %q, want %q", got, SourceProject)
	}

	names := map[string]string{}
	for _, agent := range res.Config.Agents {
		names[agent.Name] = agent.Description
	}
	if len(names) != 3 {
		t.Fatalf("agent count = %d, want 3", len(names))
	}
	if names["shared"] != "project" {
		t.Fatalf("shared agent description = %q, want project", names["shared"])
	}
	if names["project-only"] != "project" || names["user-only"] != "user" {
		t.Fatalf("unexpected agent set: %#v", names)
	}
}

func TestLoadRejectsRemovedStoresKey(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	projectFile := filepath.Join(root, "kave.yaml")
	if err := os.WriteFile(projectFile, []byte(`
stores:
  app:
    backend: sqlite
`), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}

	_, err := Load(LoadOpts{StartDir: root})
	if err == nil {
		t.Fatal("Load() error = nil, want removed stores key error")
	}
	if want := `configuration key "stores" has been removed`; !strings.Contains(err.Error(), want) {
		t.Fatalf("Load() error = %q, want substring %q", err.Error(), want)
	}
}
