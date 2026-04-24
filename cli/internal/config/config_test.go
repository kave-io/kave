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
