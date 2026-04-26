package httpbridge_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kave-io/kave/server/internal/authctx"
	"github.com/kave-io/kave/server/internal/httpbridge"
	serverauth "github.com/kave-io/kave/server/ops/auth"
)

const testKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func newTestTokenManager(t *testing.T) *serverauth.TokenManager {
	t.Helper()
	tm, err := serverauth.NewTokenManager(testKeyHex, 0, 0)
	if err != nil {
		t.Fatalf("token manager: %v", err)
	}
	return tm
}

func captureIdentity(t *testing.T, mw *httpbridge.AuthMiddleware, req *http.Request) authctx.Identity {
	t.Helper()
	var got authctx.Identity
	h := mw.Wrap(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		id, _ := authctx.From(r.Context())
		got = id
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return got
}

func TestAuthMiddleware_NoHeader_AllowAnonymous(t *testing.T) {
	mw := httpbridge.NewAuthMiddleware(nil, newTestTokenManager(t), true, false)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	id := captureIdentity(t, mw, req)
	if !id.IsAnonymous() {
		t.Fatalf("want anonymous, got %+v", id)
	}
}

func TestAuthMiddleware_NoHeader_DenyAnonymous(t *testing.T) {
	mw := httpbridge.NewAuthMiddleware(nil, newTestTokenManager(t), false, false)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	id := captureIdentity(t, mw, req)
	if !id.IsInvalid() {
		t.Fatalf("want invalid, got %+v", id)
	}
}

func TestAuthMiddleware_PASETOSession(t *testing.T) {
	tm := newTestTokenManager(t)
	tok, err := tm.IssueSession("u1", "sess1", "org1")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	mw := httpbridge.NewAuthMiddleware(nil, tm, false, false)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	id := captureIdentity(t, mw, req)
	if !id.IsUser() || id.UserID != "u1" || id.OrgID != "org1" {
		t.Fatalf("unexpected identity: %+v", id)
	}
}

func TestAuthMiddleware_PASETOAgent(t *testing.T) {
	tm := newTestTokenManager(t)
	tok, err := tm.IssueAgentToken("agn_1", "prj_1", "env_1", "org1")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	mw := httpbridge.NewAuthMiddleware(nil, tm, false, false)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer kav_"+tok)
	id := captureIdentity(t, mw, req)
	if !id.IsAgentToken() || id.AgentID != "agn_1" || id.ProjectID != "prj_1" {
		t.Fatalf("unexpected identity: %+v", id)
	}
}

func TestAuthMiddleware_BadHeader_Invalid(t *testing.T) {
	mw := httpbridge.NewAuthMiddleware(nil, newTestTokenManager(t), true, false)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Basic xyz")
	id := captureIdentity(t, mw, req)
	if !id.IsInvalid() {
		t.Fatalf("want invalid, got %+v", id)
	}
}

func TestAuthMiddleware_GarbagePASETO_Invalid(t *testing.T) {
	mw := httpbridge.NewAuthMiddleware(nil, newTestTokenManager(t), true, false)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer not-a-token")
	id := captureIdentity(t, mw, req)
	if !id.IsInvalid() {
		t.Fatalf("want invalid, got %+v", id)
	}
}
