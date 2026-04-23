package mappers

import (
	"testing"

	"github.com/kave-io/kave/core/runtime/validate"
)

func TestValidationResultToOutcome(t *testing.T) {
	result := &validate.Result{
		Valid: false,
		Violations: []validate.Violation{
			{
				Path:    "$.payload.user_id",
				Code:    "required",
				Message: "user_id is required",
			},
		},
		Retryable: false,
	}

	out := ValidationResultToOutcome(result)
	if out == nil {
		t.Fatal("expected outcome")
	}
	if out.Code != "required" {
		t.Fatalf("expected code required, got %q", out.Code)
	}
	if out.Message != "user_id is required" {
		t.Fatalf("expected message propagated, got %q", out.Message)
	}
	if out.Reason != "$.payload.user_id" {
		t.Fatalf("expected path as reason, got %q", out.Reason)
	}
}

func TestValidationResultToMetadata(t *testing.T) {
	result := &validate.Result{
		Valid:     false,
		Retryable: true,
		Violations: []validate.Violation{
			{Path: "$.x", Code: "bad_type", Message: "must be string"},
		},
	}

	meta := ValidationResultToMetadata(result)
	if meta == nil {
		t.Fatal("expected metadata")
	}
	if meta["retryable"] != true {
		t.Fatalf("expected retryable=true, got %v", meta["retryable"])
	}

	violations, ok := meta["violations"].([]map[string]any)
	if !ok || len(violations) != 1 {
		t.Fatalf("expected 1 violation entry, got %T %+v", meta["violations"], meta["violations"])
	}
}

func TestValidationResultToOutcomeValidReturnsNil(t *testing.T) {
	if out := ValidationResultToOutcome(&validate.Result{Valid: true}); out != nil {
		t.Fatalf("expected nil outcome, got %+v", out)
	}
}
