package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesSPARoutesWithProductionHeaders(t *testing.T) {
	t.Parallel()
	response := httptest.NewRecorder()
	Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/tenants", nil))

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `<div id="app"`) {
		t.Fatalf("SPA response = %d %q", response.Code, response.Body.String())
	}
	for name, want := range map[string]string{
		"Cache-Control":                "no-store",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Referrer-Policy":              "no-referrer",
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
	} {
		if got := response.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	policy := response.Header().Get("Content-Security-Policy")
	if !strings.Contains(policy, "frame-ancestors 'none'") || !strings.Contains(policy, "connect-src 'self'") {
		t.Fatalf("CSP = %q", policy)
	}
}

func TestHandlerDoesNotFallBackForMissingAssetsOrUnsafeMethods(t *testing.T) {
	t.Parallel()
	handler := Handler()

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	if missing.Code != http.StatusNotFound || strings.Contains(missing.Body.String(), `<div id="app"`) {
		t.Fatalf("missing asset = %d %q", missing.Code, missing.Body.String())
	}

	post := httptest.NewRecorder()
	handler.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/", nil))
	if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST = %d allow=%q", post.Code, post.Header().Get("Allow"))
	}
}
