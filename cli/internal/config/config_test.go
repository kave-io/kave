package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRejectsRemovedStoresKey(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	configPath := filepath.Join(root, ConfigNameYAML)
	if err := os.WriteFile(configPath, []byte(`
stores:
  app:
    backend: sqlite
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Resolve(RootOptions{ConfigPath: configPath})
	if err == nil {
		t.Fatal("Resolve() error = nil, want removed stores key error")
	}
	if want := `configuration key "stores" has been removed`; !strings.Contains(err.Error(), want) {
		t.Fatalf("Resolve() error = %q, want substring %q", err.Error(), want)
	}
}

func TestResolveLoadsStorageKey(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	configPath := filepath.Join(root, ConfigNameYAML)
	if err := os.WriteFile(configPath, []byte(`
storage:
  defaults:
    app:
      kind: sqlite
      path: app.db
    span:
      kind: duckdb
      path: spans.duckdb
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	res, err := Resolve(RootOptions{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if res.LoadedConfig == nil || res.LoadedConfig.Storage == nil {
		t.Fatal("LoadedConfig.Storage = nil, want parsed storage config")
	}
	if got := res.LoadedConfig.Storage.Defaults.App.Path; got != "app.db" {
		t.Fatalf("storage.defaults.app.path = %q, want app.db", got)
	}
	if got := res.LoadedConfig.Storage.Defaults.Span.Path; got != "spans.duckdb" {
		t.Fatalf("storage.defaults.span.path = %q, want spans.duckdb", got)
	}
}

func TestResolvePrefersUserConfigOverDiscoveredProjectConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KAVE_CONFIG", "")

	userDir := filepath.Join(home, ".kave")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("create user dir: %v", err)
	}

	userConfigPath := filepath.Join(userDir, ConfigNameYAML)
	if err := os.WriteFile(userConfigPath, []byte(`
contexts:
  - name: default
    server: 127.0.0.1:19090
currentContext: default
`), 0o644); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	projectDir := t.TempDir()
	projectConfigPath := filepath.Join(projectDir, ConfigNameYAML)
	if err := os.WriteFile(projectConfigPath, []byte(`
contexts:
  - name: default
    server: 127.0.0.1:7777
currentContext: default
`), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir project: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	res, err := Resolve(RootOptions{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got := res.ConfigPath; got != userConfigPath {
		t.Fatalf("ConfigPath = %q, want %q", got, userConfigPath)
	}
	if got := res.ActiveServer(); got != "127.0.0.1:19090" {
		t.Fatalf("ActiveServer() = %q, want 127.0.0.1:19090", got)
	}
}
