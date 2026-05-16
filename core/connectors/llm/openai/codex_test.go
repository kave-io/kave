package openai

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	connruntime "github.com/kave-io/kave/core/connectors/runtime"
)

func writeCodexFixture(t *testing.T, accessToken, accountID string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	payload := `{"tokens":{"access_token":"` + accessToken + `","account_id":"` + accountID + `"}}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func codexCall(t *testing.T) *connruntime.LLMCall {
	t.Helper()
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	return &connruntime.LLMCall{
		Provider:        "openai",
		Method:          "POST",
		UpstreamBaseURL: "https://chatgpt.com",
		UpstreamPath:    "/backend-api/codex/responses",
		Header:          h,
		Body:            []byte(`{"model":"gpt-5-codex","input":"hi"}`),
	}
}

func TestCodexSelfAcquireCredential(t *testing.T) {
	t.Setenv("KAVE_CODEX_AUTH_PATH", writeCodexFixture(t, "tok-self", "acct-self"))
	conn := NewConnector(nil)

	prep, err := conn.PrepareRequest(codexCall(t), "")
	if err != nil {
		t.Fatalf("PrepareRequest: %v", err)
	}
	if got, want := prep.Header.Get("Authorization"), "Bearer tok-self"; got != want {
		t.Errorf("Authorization=%q want %q", got, want)
	}
	if got := prep.Header.Get("chatgpt-account-id"); got != "acct-self" {
		t.Errorf("chatgpt-account-id=%q want acct-self", got)
	}
	if got, want := prep.Header.Get("OpenAI-Beta"), "responses=experimental"; got != want {
		t.Errorf("OpenAI-Beta=%q want %q", got, want)
	}
	if prep.Header.Get("originator") != "codex_cli_rs" {
		t.Errorf("originator header not injected")
	}
	if prep.Header.Get("version") == "" {
		t.Errorf("version header not injected")
	}
	if got, want := prep.URL, "https://chatgpt.com/backend-api/codex/responses"; got != want {
		t.Errorf("URL=%q want %q", got, want)
	}
}

func TestCodexMissingAuthReturnsCredentialRequired(t *testing.T) {
	t.Setenv("KAVE_CODEX_AUTH_PATH", filepath.Join(t.TempDir(), "missing.json"))
	conn := NewConnector(nil)

	_, err := conn.PrepareRequest(codexCall(t), "")
	if !errors.Is(err, connruntime.ErrCredentialRequired) {
		t.Fatalf("err=%v want ErrCredentialRequired", err)
	}
}

func TestCodexExplicitCredentialSkipsSelfAcquire(t *testing.T) {
	// Point at a malformed file: if the connector tried to load it the test
	// would fail. Supplying a credential must short-circuit the load entirely.
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("garbage"), 0o600); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	t.Setenv("KAVE_CODEX_AUTH_PATH", bad)
	conn := NewConnector(nil)

	prep, err := conn.PrepareRequest(codexCall(t), "explicit-token")
	if err != nil {
		t.Fatalf("PrepareRequest: %v", err)
	}
	if got, want := prep.Header.Get("Authorization"), "Bearer explicit-token"; got != want {
		t.Errorf("Authorization=%q want %q", got, want)
	}
	// Self-acquire branch is skipped, so magic Codex headers should NOT be
	// injected. The gateway is expected to forward whatever the caller sent.
	if prep.Header.Get("chatgpt-account-id") != "" {
		t.Errorf("explicit credential path should not inject chatgpt-account-id")
	}
	if prep.Header.Get("originator") != "" {
		t.Errorf("explicit credential path should not inject originator")
	}
}

func TestCodexPreservesCallerAccountID(t *testing.T) {
	t.Setenv("KAVE_CODEX_AUTH_PATH", writeCodexFixture(t, "tok-self", "acct-from-file"))
	conn := NewConnector(nil)

	call := codexCall(t)
	call.Header.Set("chatgpt-account-id", "acct-from-caller")
	prep, err := conn.PrepareRequest(call, "")
	if err != nil {
		t.Fatalf("PrepareRequest: %v", err)
	}
	if got := prep.Header.Get("chatgpt-account-id"); got != "acct-from-caller" {
		t.Errorf("chatgpt-account-id=%q want caller value preserved", got)
	}
}

func TestCodexNonCodexPathSkipsBranch(t *testing.T) {
	t.Setenv("KAVE_CODEX_AUTH_PATH", filepath.Join(t.TempDir(), "missing.json"))
	conn := NewConnector(nil)

	call := codexCall(t)
	call.UpstreamPath = "/v1/responses"
	call.UpstreamBaseURL = ""
	// No credential, non-codex path: connector forwards without auth instead
	// of trying to load codex auth (which would fail).
	prep, err := conn.PrepareRequest(call, "")
	if err != nil {
		t.Fatalf("PrepareRequest: %v", err)
	}
	if got := prep.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization=%q want empty for non-codex unauthenticated call", got)
	}
}
