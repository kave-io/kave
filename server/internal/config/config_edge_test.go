package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── Expand edge cases not covered by expand_test.go ────────────────────────

func TestExpand_unsetVarNoDefault(t *testing.T) {
	_, err := Expand("value: ${UNSET_VAR}", "cfg.yaml", map[string]string{})
	if err == nil {
		t.Fatal("expected error for unset var without default")
	}
	if !strings.Contains(err.Error(), "UNSET_VAR") {
		t.Errorf("error should mention the var name: %v", err)
	}
}

func TestExpand_setVarIsUsed(t *testing.T) {
	got, err := Expand("host: ${HOST}", "cfg.yaml", map[string]string{"HOST": "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "localhost") {
		t.Errorf("expected expanded value, got %q", got)
	}
}

func TestExpand_defaultUsedWhenVarEmpty(t *testing.T) {
	// Empty string in env → uses default
	got, err := Expand("port: ${PORT:-8080}", "cfg.yaml", map[string]string{"PORT": ""})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "8080") {
		t.Errorf("expected default 8080, got %q", got)
	}
}

func TestExpand_defaultNotUsedWhenVarSet(t *testing.T) {
	got, err := Expand("port: ${PORT:-8080}", "cfg.yaml", map[string]string{"PORT": "9090"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "8080") {
		t.Errorf("default should not be used when var is set; got %q", got)
	}
	if !strings.Contains(got, "9090") {
		t.Errorf("expected 9090, got %q", got)
	}
}

func TestExpand_requiredWithMessage(t *testing.T) {
	_, err := Expand("key: ${DB_URL:?DATABASE_URL must be set}", "cfg.yaml", map[string]string{})
	if err == nil {
		t.Fatal("expected error for required var")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL must be set") {
		t.Errorf("error should include the custom message: %v", err)
	}
}

func TestExpand_emptyVarName(t *testing.T) {
	_, err := Expand("key: ${}", "cfg.yaml", map[string]string{})
	if err == nil {
		t.Fatal("expected error for empty variable name")
	}
}

func TestExpand_unclosedBrace_silentPassthrough(t *testing.T) {
	// Design choice: unclosed ${... with no closing } keeps the literal $
	got, err := Expand("key: ${UNCLOSED", "cfg.yaml", map[string]string{})
	if err != nil {
		t.Fatalf("unclosed brace should not error, got: %v", err)
	}
	if !strings.Contains(got, "$") {
		t.Errorf("expected literal $ for unclosed brace, got %q", got)
	}
}

func TestExpand_multipleVarsOnOneLine(t *testing.T) {
	env := map[string]string{"A": "alpha", "B": "beta"}
	got, err := Expand("line: ${A}-${B}", "cfg.yaml", env)
	if err != nil {
		t.Fatal(err)
	}
	if got != "line: alpha-beta" {
		t.Errorf("got %q", got)
	}
}

func TestExpand_multilinePreservesStructure(t *testing.T) {
	env := map[string]string{"HOST": "db.local"}
	input := "a: 1\nhost: ${HOST}\nb: 2"
	got, err := Expand(input, "cfg.yaml", env)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), got)
	}
	if lines[1] != "host: db.local" {
		t.Errorf("line 2 mismatch: %q", lines[1])
	}
}

func TestExpand_doubleEscape(t *testing.T) {
	got, err := Expand("literal: $$VAR", "cfg.yaml", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "literal: $VAR" {
		t.Errorf("expected literal $VAR, got %q", got)
	}
}

// ── Layered config edge cases ───────────────────────────────────────────────

func TestLoad_invalidYAML(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	if err := os.WriteFile(filepath.Join(root, "kave.yaml"), []byte("not: valid: yaml: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(LoadOpts{StartDir: root})
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoad_missingStartDir_noError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// No kave.yaml anywhere — should succeed with defaults
	res, err := Load(LoadOpts{StartDir: t.TempDir()})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res == nil || res.Config == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestLoad_envOverridePrecedence(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	// Project file sets refresh interval to 5
	if err := os.WriteFile(filepath.Join(root, "kave.yaml"), []byte("fx:\n  refresh_interval_seconds: 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Env overrides to 99
	res, err := Load(LoadOpts{
		StartDir: root,
		Env:      map[string]string{"KAVE_FX_REFRESH_INTERVAL_SECONDS": "99"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Config.FX.RefreshIntervalSeconds != 99 {
		t.Errorf("env should override project: got %d, want 99", res.Config.FX.RefreshIntervalSeconds)
	}
	if res.Origin["fx.refresh_interval_seconds"] != SourceEnv {
		t.Errorf("origin should be env, got %q", res.Origin["fx.refresh_interval_seconds"])
	}
}

func TestLoad_userOverridesProject(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	userDir := filepath.Join(home, ".kave")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Project sets app path to project.db
	if err := os.WriteFile(filepath.Join(root, "kave.yaml"), []byte(`
storage:
  defaults:
    app:
      path: project.db
`), 0o644); err != nil {
		t.Fatal(err)
	}
	// User overrides app kind to postgres
	if err := os.WriteFile(filepath.Join(userDir, "kave.yaml"), []byte(`
storage:
  defaults:
    app:
      kind: postgres
`), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Load(LoadOpts{StartDir: root})
	if err != nil {
		t.Fatal(err)
	}
	// Project wins for path (not in user file)
	if res.Config.Storage.Defaults.App.Path != "project.db" {
		t.Errorf("project app path should be preserved: %q", res.Config.Storage.Defaults.App.Path)
	}
	// User wins for kind
	if res.Config.Storage.Defaults.App.Kind != "postgres" {
		t.Errorf("user app kind should win: %q", res.Config.Storage.Defaults.App.Kind)
	}
}

func TestLoad_layerCountWithProjectAndUser(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	userDir := filepath.Join(home, ".kave")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "kave.yaml"), []byte("fx:\n  refresh_interval_seconds: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "kave.yaml"), []byte("fx:\n  refresh_interval_seconds: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Load(LoadOpts{StartDir: root})
	if err != nil {
		t.Fatal(err)
	}
	// At least builtin + user + project layers
	if len(res.Layers) < 3 {
		t.Errorf("expected at least 3 layers (builtin, user, project), got %d", len(res.Layers))
	}
}

func TestLoad_removedStoresKey_errorMessage(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	if err := os.WriteFile(filepath.Join(root, "kave.yaml"), []byte("stores:\n  app:\n    backend: sqlite\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(LoadOpts{StartDir: root})
	if err == nil {
		t.Fatal("expected error for removed 'stores' key")
	}
	if !strings.Contains(err.Error(), "stores") {
		t.Errorf("error should mention 'stores': %v", err)
	}
	if !strings.Contains(err.Error(), "storage.defaults") {
		t.Errorf("error should mention replacement path: %v", err)
	}
}

func TestLoad_agentListMerge_projectWinsForShared(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	userDir := filepath.Join(home, ".kave")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "kave.yaml"), []byte(`
agents:
  - name: shared
    description: from-project
    env: dev
    policy: strict
    credentials: [a]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "kave.yaml"), []byte(`
agents:
  - name: shared
    description: from-user
    env: dev
    policy: exploratory
    credentials: [b]
  - name: user-only
    description: user
    env: dev
    policy: exploratory
    credentials: [c]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Load(LoadOpts{StartDir: root})
	if err != nil {
		t.Fatal(err)
	}

	byName := map[string]string{}
	for _, a := range res.Config.Agents {
		byName[a.Name] = a.Description
	}
	if byName["shared"] != "from-project" {
		t.Errorf("project should win for shared agent name; got %q", byName["shared"])
	}
	if byName["user-only"] != "user" {
		t.Errorf("user-only agent should be present; got %q", byName["user-only"])
	}
}
