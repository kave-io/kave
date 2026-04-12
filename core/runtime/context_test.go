package runtime_test

import (
	"context"
	"testing"

	"github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/runtime/policy"
)

func TestValidationResult_RoundTrip(t *testing.T) {
	ctx := context.Background()
	vr := &runtime.ValidationResult{
		Valid:            true,
		ViolationCount:   5,
		EnforcementMode:  "block",
		ValidatorName:    "schema-validator",
		ValidatorVersion: "1.2.3",
		RuleVersion:      "2024-01",
		DurationMs:       150,
	}

	ctx = runtime.WithValidationResult(ctx, vr)
	got := runtime.ValidationResultFrom(ctx)

	if got == nil {
		t.Fatal("ValidationResultFrom returned nil, expected ValidationResult")
	}
	if got.Valid != vr.Valid {
		t.Errorf("Valid: got %v, want %v", got.Valid, vr.Valid)
	}
	if got.ViolationCount != vr.ViolationCount {
		t.Errorf("ViolationCount: got %d, want %d", got.ViolationCount, vr.ViolationCount)
	}
	if got.EnforcementMode != vr.EnforcementMode {
		t.Errorf("EnforcementMode: got %q, want %q", got.EnforcementMode, vr.EnforcementMode)
	}
	if got.ValidatorName != vr.ValidatorName {
		t.Errorf("ValidatorName: got %q, want %q", got.ValidatorName, vr.ValidatorName)
	}
	if got.ValidatorVersion != vr.ValidatorVersion {
		t.Errorf("ValidatorVersion: got %q, want %q", got.ValidatorVersion, vr.ValidatorVersion)
	}
	if got.RuleVersion != vr.RuleVersion {
		t.Errorf("RuleVersion: got %q, want %q", got.RuleVersion, vr.RuleVersion)
	}
	if got.DurationMs != vr.DurationMs {
		t.Errorf("DurationMs: got %d, want %d", got.DurationMs, vr.DurationMs)
	}
}

func TestValidationResult_NilReturnedFromEmpty(t *testing.T) {
	ctx := context.Background()
	got := runtime.ValidationResultFrom(ctx)
	if got != nil {
		t.Fatalf("ValidationResultFrom on empty context should return nil, got %v", got)
	}
}

func TestValidationResult_NilStored(t *testing.T) {
	ctx := context.Background()
	ctx = runtime.WithValidationResult(ctx, nil)
	got := runtime.ValidationResultFrom(ctx)
	if got != nil {
		t.Fatalf("ValidationResultFrom after storing nil should return nil, got %v", got)
	}
}

func TestValidationResult_CoexistsWithOtherKeys(t *testing.T) {
	ctx := context.Background()

	// Store multiple context values
	p := &policy.Policy{ID: "p-1"}
	ctx = runtime.WithPolicy(ctx, p)

	r := &runtime.Run{ID: "r-1"}
	ctx = runtime.WithRun(ctx, r)

	tu := &runtime.TokenUsage{InputTokens: 10, OutputTokens: 20}
	ctx = runtime.WithTokenUsage(ctx, tu)

	u := &runtime.Usage{RequestCount: 5}
	ctx = runtime.WithUsage(ctx, u)

	vr := &runtime.ValidationResult{Valid: true, ValidatorName: "test"}
	ctx = runtime.WithValidationResult(ctx, vr)

	// Verify all coexist
	if pGot := runtime.PolicyFrom(ctx); pGot == nil || pGot.ID != "p-1" {
		t.Fatalf("Policy lost: got %v", pGot)
	}
	if rGot := runtime.RunFrom(ctx); rGot == nil || rGot.ID != "r-1" {
		t.Fatalf("Run lost: got %v", rGot)
	}
	if tuGot := runtime.TokenUsageFrom(ctx); tuGot == nil || tuGot.InputTokens != 10 {
		t.Fatalf("TokenUsage lost: got %v", tuGot)
	}
	if uGot := runtime.UsageFrom(ctx); uGot == nil || uGot.RequestCount != 5 {
		t.Fatalf("Usage lost: got %v", uGot)
	}
	if vrGot := runtime.ValidationResultFrom(ctx); vrGot == nil || vrGot.ValidatorName != "test" {
		t.Fatalf("ValidationResult lost: got %v", vrGot)
	}
}

func TestValidationResult_OverwriteReplacesPrevious(t *testing.T) {
	ctx := context.Background()

	vr1 := &runtime.ValidationResult{Valid: true, ValidatorName: "v1"}
	ctx = runtime.WithValidationResult(ctx, vr1)

	vr2 := &runtime.ValidationResult{Valid: false, ValidatorName: "v2"}
	ctx = runtime.WithValidationResult(ctx, vr2)

	got := runtime.ValidationResultFrom(ctx)
	if got == nil {
		t.Fatal("ValidationResultFrom returned nil")
	}
	if got.ValidatorName != "v2" {
		t.Errorf("expected second ValidationResult, got ValidatorName %q", got.ValidatorName)
	}
	if got.Valid != false {
		t.Errorf("expected Valid=false from second, got %v", got.Valid)
	}
}

func TestValidationResult_Fields(t *testing.T) {
	tests := []struct {
		name string
		vr   *runtime.ValidationResult
	}{
		{
			name: "all fields set",
			vr: &runtime.ValidationResult{
				Valid:            true,
				ViolationCount:   3,
				EnforcementMode:  "warn",
				ValidatorName:    "openapi-validator",
				ValidatorVersion: "2.5.0",
				RuleVersion:      "2025-04",
				DurationMs:       250,
			},
		},
		{
			name: "minimal fields",
			vr: &runtime.ValidationResult{
				Valid: false,
			},
		},
		{
			name: "zero DurationMs",
			vr: &runtime.ValidationResult{
				Valid:            true,
				DurationMs:       0,
				ValidatorName:    "test",
				EnforcementMode:  "block",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := runtime.WithValidationResult(context.Background(), tt.vr)
			got := runtime.ValidationResultFrom(ctx)

			if got == nil {
				t.Fatal("round-trip returned nil")
			}
			if got.Valid != tt.vr.Valid {
				t.Errorf("Valid mismatch: got %v, want %v", got.Valid, tt.vr.Valid)
			}
			if got.ViolationCount != tt.vr.ViolationCount {
				t.Errorf("ViolationCount mismatch: got %d, want %d", got.ViolationCount, tt.vr.ViolationCount)
			}
			if got.EnforcementMode != tt.vr.EnforcementMode {
				t.Errorf("EnforcementMode mismatch: got %q, want %q", got.EnforcementMode, tt.vr.EnforcementMode)
			}
			if got.ValidatorName != tt.vr.ValidatorName {
				t.Errorf("ValidatorName mismatch: got %q, want %q", got.ValidatorName, tt.vr.ValidatorName)
			}
			if got.ValidatorVersion != tt.vr.ValidatorVersion {
				t.Errorf("ValidatorVersion mismatch: got %q, want %q", got.ValidatorVersion, tt.vr.ValidatorVersion)
			}
			if got.RuleVersion != tt.vr.RuleVersion {
				t.Errorf("RuleVersion mismatch: got %q, want %q", got.RuleVersion, tt.vr.RuleVersion)
			}
			if got.DurationMs != tt.vr.DurationMs {
				t.Errorf("DurationMs mismatch: got %d, want %d", got.DurationMs, tt.vr.DurationMs)
			}
		})
	}
}
