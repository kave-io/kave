package mappers

import (
	"testing"

	controlmodel "github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/runtime/policy"
)

func TestRecordToPolicy_UsesAllowedTypesAndBudgetCap(t *testing.T) {
	rec := &controlmodel.PolicyRecord{
		ID:                "p-1",
		ProjectID:         "proj-1",
		Name:              "base",
		AllowedTypes:      []string{"llm"},
		AllowedConnectors: []string{"openai"},
		AllowedMethods:    []string{"responses.create"},
		BudgetCap:         money.MustParseDollars("12.75"),
		Mode:              string(policy.ModeEnforce),
		Status:            string(controlmodel.PolicyStatusActive),
	}

	out := RecordToPolicy(rec)
	if out == nil || out.Auth == nil || out.Cost == nil {
		t.Fatal("expected non-nil policy with auth and cost")
	}
	if len(out.Auth.AllowedTypes) != 1 || out.Auth.AllowedTypes[0] != "llm" {
		t.Fatalf("expected allowed types from record, got %+v", out.Auth.AllowedTypes)
	}

	want := money.MustParseDollars("12.75")
	if out.Cost.BudgetCap == nil || *out.Cost.BudgetCap != want {
		t.Fatalf("expected budget cap %v, got %v", want, out.Cost.BudgetCap)
	}
}

func TestPolicyToRecord_PreservesAllowedTypesAndBudgetCap(t *testing.T) {
	cap := money.MustParseDollars("3.5")
	p := &policy.Policy{
		ID:        "p-2",
		ProjectID: "proj-2",
		Name:      "strict",
		Auth: &policy.AuthPolicy{
			AllowedTypes:      []string{"api"},
			AllowedConnectors: []string{"github"},
			AllowedMethods:    []string{"pulls.list"},
		},
		Cost: &policy.CostPolicy{
			BudgetCap: &cap,
		},
	}

	rec := PolicyToRecord(p)
	if rec == nil {
		t.Fatal("expected record")
	}
	if len(rec.AllowedTypes) != 1 || rec.AllowedTypes[0] != "api" {
		t.Fatalf("expected allowed types, got %+v", rec.AllowedTypes)
	}
	if rec.BudgetCap != money.MustParseDollars("3.5") {
		t.Fatalf("expected budget cap %v, got %v", money.MustParseDollars("3.5"), rec.BudgetCap)
	}
}
