package auth_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/kave-io/kave/server/internal/authctx"
	"github.com/kave-io/kave/server/ops/auth"
)

func randomKeyHex(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return hex.EncodeToString(b)
}

// TestPASETO_AgentToken_RoundTrip verifies that an issued agent PASETO token
// round-trips through Verify with the correct identity fields intact.
func TestPASETO_AgentToken_RoundTrip(t *testing.T) {
	keyHex := randomKeyHex(t)
	m, err := auth.NewTokenManager(keyHex, time.Hour, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("NewTokenManager: %v", err)
	}
	if !m.Enabled() {
		t.Fatal("expected token manager to be enabled with a key")
	}

	token, err := m.IssueAgentToken("agt-test", "prj-test", "env-test", "org-test")
	if err != nil {
		t.Fatalf("IssueAgentToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	id, err := m.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id.Kind != authctx.KindAgent {
		t.Errorf("Kind = %q, want %q", id.Kind, authctx.KindAgent)
	}
	if id.AgentID != "agt-test" {
		t.Errorf("AgentID = %q, want agt-test", id.AgentID)
	}
	if id.ProjectID != "prj-test" {
		t.Errorf("ProjectID = %q, want prj-test", id.ProjectID)
	}
	if id.EnvID != "env-test" {
		t.Errorf("EnvID = %q, want env-test", id.EnvID)
	}
	if id.OrgID != "org-test" {
		t.Errorf("OrgID = %q, want org-test", id.OrgID)
	}
}

// TestPASETO_SessionToken_RoundTrip verifies that an issued session PASETO
// token round-trips with user/session identity.
func TestPASETO_SessionToken_RoundTrip(t *testing.T) {
	keyHex := randomKeyHex(t)
	m, err := auth.NewTokenManager(keyHex, time.Hour, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("NewTokenManager: %v", err)
	}

	token, err := m.IssueSession("usr-test", "sess-test", "org-test")
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}

	id, err := m.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id.Kind != authctx.KindUser {
		t.Errorf("Kind = %q, want %q", id.Kind, authctx.KindUser)
	}
	if id.UserID != "usr-test" {
		t.Errorf("UserID = %q, want usr-test", id.UserID)
	}
	if id.SessionID != "sess-test" {
		t.Errorf("SessionID = %q, want sess-test", id.SessionID)
	}
}

// TestPASETO_WrongKey verifies that a token issued with one key cannot be
// verified with a different key.
func TestPASETO_WrongKey(t *testing.T) {
	m1, _ := auth.NewTokenManager(randomKeyHex(t), time.Hour, time.Hour)
	m2, _ := auth.NewTokenManager(randomKeyHex(t), time.Hour, time.Hour)

	token, err := m1.IssueAgentToken("agt", "prj", "env", "org")
	if err != nil {
		t.Fatalf("IssueAgentToken: %v", err)
	}

	if _, err := m2.Verify(token); err == nil {
		t.Error("expected Verify to fail with wrong key, got nil error")
	}
}

// TestPASETO_Disabled verifies behavior when no key is configured.
func TestPASETO_Disabled(t *testing.T) {
	m, err := auth.NewTokenManager("", time.Hour, time.Hour)
	if err != nil {
		t.Fatalf("NewTokenManager: %v", err)
	}
	if m.Enabled() {
		t.Error("expected manager to be disabled when no key")
	}

	// IssueAgentToken falls back to opaque tokens when disabled.
	token, err := m.IssueAgentToken("agt", "prj", "env", "org")
	if err != nil {
		t.Fatalf("IssueAgentToken disabled: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty fallback token")
	}

	// Verify returns ErrTokensDisabled when no key.
	if _, err := m.Verify(token); err != auth.ErrTokensDisabled {
		t.Errorf("Verify disabled: got %v, want ErrTokensDisabled", err)
	}
}

// TestPASETO_ParseIdentity_AgentToken verifies that ParseIdentity correctly
// resolves a PASETO agent token into an Agent identity.
func TestPASETO_ParseIdentity_AgentToken(t *testing.T) {
	keyHex := randomKeyHex(t)
	m, _ := auth.NewTokenManager(keyHex, time.Hour, time.Hour)

	token, err := m.IssueAgentToken("agt-pi", "prj-pi", "env-pi", "org-pi")
	if err != nil {
		t.Fatalf("IssueAgentToken: %v", err)
	}

	id, err := auth.ParseIdentity(context.Background(), "Bearer "+token, nil, m, false)
	if err != nil {
		t.Fatalf("ParseIdentity: %v", err)
	}
	if !id.IsAgentToken() {
		t.Errorf("expected agent identity, got kind=%q", id.Kind)
	}
	if id.AgentID != "agt-pi" {
		t.Errorf("AgentID = %q, want agt-pi", id.AgentID)
	}
}
