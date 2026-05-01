package gateway

import (
	"errors"
	"fmt"
	"testing"

	serverauth "github.com/kave-io/kave/server/ops/auth"
	serverbudget "github.com/kave-io/kave/server/ops/budget"
	serverpolicy "github.com/kave-io/kave/server/ops/policy"
)

func TestMapError_policyBlocked(t *testing.T) {
	status, code, _, _ := mapError(serverpolicy.ErrPolicyBlocked)
	if status != 403 {
		t.Errorf("expected 403, got %d", status)
	}
	if code != "gateway.policy_blocked" {
		t.Errorf("expected gateway.policy_blocked, got %q", code)
	}
}

func TestMapError_policyBlocked_wrappedBlockedError(t *testing.T) {
	err := &serverpolicy.BlockedError{
		Reason:  "agent not allowed",
		Subject: "agt_123",
		Object:  "/v1/chat",
	}
	status, code, msg, details := mapError(fmt.Errorf("pipeline: %w", err))
	if status != 403 {
		t.Errorf("expected 403, got %d", status)
	}
	if code != "gateway.policy_blocked" {
		t.Errorf("expected gateway.policy_blocked, got %q", code)
	}
	if msg != "agent not allowed" {
		t.Errorf("expected reason as message, got %q", msg)
	}
	if details["subject"] != "agt_123" {
		t.Errorf("expected subject in details, got %v", details)
	}
}

func TestMapError_budgetExceeded(t *testing.T) {
	status, code, _, _ := mapError(serverbudget.ErrBudgetExceeded)
	if status != 402 {
		t.Errorf("expected 402, got %d", status)
	}
	if code != "gateway.budget_exceeded" {
		t.Errorf("expected gateway.budget_exceeded, got %q", code)
	}
}

func TestMapError_budgetExceeded_details(t *testing.T) {
	err := &serverbudget.ExceededError{
		Period:  "monthly",
		Subject: "agt_abc",
	}
	status, code, _, details := mapError(fmt.Errorf("wrap: %w", err))
	if status != 402 {
		t.Errorf("expected 402, got %d", status)
	}
	if code != "gateway.budget_exceeded" {
		t.Errorf("expected gateway.budget_exceeded, got %q", code)
	}
	if details["period"] != "monthly" {
		t.Errorf("expected period=monthly in details, got %v", details)
	}
}

func TestMapError_quotaExceeded(t *testing.T) {
	status, code, _, _ := mapError(ErrQuotaExceeded)
	if status != 429 {
		t.Errorf("expected 429, got %d", status)
	}
	if code != "gateway.quota_exceeded" {
		t.Errorf("expected gateway.quota_exceeded, got %q", code)
	}
}

func TestMapError_unauthenticated(t *testing.T) {
	status, code, _, _ := mapError(serverauth.ErrUnauthenticated)
	if status != 401 {
		t.Errorf("expected 401, got %d", status)
	}
	if code != "gateway.unauthorized" {
		t.Errorf("expected gateway.unauthorized, got %q", code)
	}
}

func TestMapError_unauthorized(t *testing.T) {
	status, code, _, _ := mapError(serverauth.ErrUnauthorized)
	if status != 403 {
		t.Errorf("expected 403, got %d", status)
	}
	if code != "gateway.policy_blocked" {
		t.Errorf("expected gateway.policy_blocked, got %q", code)
	}
}

func TestMapError_providerNotFound(t *testing.T) {
	status, code, _, _ := mapError(ErrProviderNotFound)
	if status != 400 {
		t.Errorf("expected 400, got %d", status)
	}
	if code != "gateway.provider_not_supported" {
		t.Errorf("expected gateway.provider_not_supported, got %q", code)
	}
}

func TestMapError_upstream(t *testing.T) {
	status, code, _, _ := mapError(ErrUpstream)
	if status != 502 {
		t.Errorf("expected 502, got %d", status)
	}
	if code != "gateway.upstream_error" {
		t.Errorf("expected gateway.upstream_error, got %q", code)
	}
}

func TestMapError_unknown(t *testing.T) {
	status, code, _, _ := mapError(errors.New("some unexpected error"))
	if status != 500 {
		t.Errorf("expected 500, got %d", status)
	}
	if code != "gateway.internal_error" {
		t.Errorf("expected gateway.internal_error, got %q", code)
	}
}

func TestMapError_wrappedSentinels(t *testing.T) {
	// Wrapped errors must still match sentinels via errors.Is
	cases := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{fmt.Errorf("wrap: %w", ErrQuotaExceeded), 429, "gateway.quota_exceeded"},
		{fmt.Errorf("wrap: %w", ErrProviderNotFound), 400, "gateway.provider_not_supported"},
		{fmt.Errorf("wrap: %w", ErrUpstream), 502, "gateway.upstream_error"},
		{fmt.Errorf("wrap: %w", serverauth.ErrUnauthenticated), 401, "gateway.unauthorized"},
	}
	for _, tc := range cases {
		status, code, _, _ := mapError(tc.err)
		if status != tc.wantStatus || code != tc.wantCode {
			t.Errorf("mapError(%v): got (%d, %q), want (%d, %q)", tc.err, status, code, tc.wantStatus, tc.wantCode)
		}
	}
}
