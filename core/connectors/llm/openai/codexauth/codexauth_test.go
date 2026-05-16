package codexauth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeAuth(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestLoadHappyPath(t *testing.T) {
	path := writeAuth(t, `{"tokens":{"access_token":"tok-abc","account_id":"acct-123"}}`)
	t.Setenv("KAVE_CODEX_AUTH_PATH", path)

	auth, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if auth.AccessToken != "tok-abc" {
		t.Errorf("AccessToken=%q want tok-abc", auth.AccessToken)
	}
	if auth.AccountID != "acct-123" {
		t.Errorf("AccountID=%q want acct-123", auth.AccountID)
	}
}

func TestLoadMissingFileReturnsNotConfigured(t *testing.T) {
	t.Setenv("KAVE_CODEX_AUTH_PATH", filepath.Join(t.TempDir(), "absent.json"))
	_, err := Load()
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("err=%v want ErrNotConfigured", err)
	}
}

func TestLoadEmptyTokenReturnsNotConfigured(t *testing.T) {
	path := writeAuth(t, `{"tokens":{"access_token":"","account_id":"acct"}}`)
	t.Setenv("KAVE_CODEX_AUTH_PATH", path)

	_, err := Load()
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("err=%v want ErrNotConfigured", err)
	}
}

func TestLoadMalformedJSONReturnsParseError(t *testing.T) {
	path := writeAuth(t, `not json {`)
	t.Setenv("KAVE_CODEX_AUTH_PATH", path)

	_, err := Load()
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if errors.Is(err, ErrNotConfigured) {
		t.Errorf("parse error must NOT be classified as ErrNotConfigured")
	}
}

func TestLoadAccountIDOptional(t *testing.T) {
	path := writeAuth(t, `{"tokens":{"access_token":"tok-only"}}`)
	t.Setenv("KAVE_CODEX_AUTH_PATH", path)

	auth, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if auth.AccessToken != "tok-only" || auth.AccountID != "" {
		t.Errorf("auth=%+v want AccessToken=tok-only AccountID=\"\"", auth)
	}
}
