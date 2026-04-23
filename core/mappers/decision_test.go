package mappers

import (
	"testing"

	"github.com/kave-io/kave/core/runtime/auth"
)

func TestAuthDecisionToOutcomeDefaults(t *testing.T) {
	out := AuthDecisionToOutcome(&auth.Decision{Allowed: false})
	if out == nil {
		t.Fatal("expected outcome")
	}
	if out.Code != "deny" {
		t.Fatalf("expected deny code, got %q", out.Code)
	}
	if out.Message != "denied" {
		t.Fatalf("expected denied message, got %q", out.Message)
	}
}
