package auth

import (
	"context"
	"testing"

	"github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/runtime/policy"
)

func TestAllowAll(t *testing.T) {
	engine := AllowAll{}
	action := &runtime.Action{
		Invocation: runtime.Invocation{
			InvocationTarget: runtime.InvocationTarget{
				Type:      runtime.TypeLLM,
				Connector: "openai",
				Method:    "chat.completions",
			},
		},
	}
	authPolicy := &policy.AuthPolicy{
		PolicyID:          "p-1",
		AllowedTypes:      []string{"llm"},
		AllowedConnectors: []string{"openai"},
		AllowedMethods:    []string{"chat.completions"},
	}

	decision, err := engine.Evaluate(context.Background(), action, authPolicy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision == nil || !decision.Allowed {
		t.Fatal("AllowAll should always return an allowed decision")
	}
	if decision.Code != "allow_all" {
		t.Fatalf("unexpected code: %q", decision.Code)
	}
	if decision.PolicyID == nil || *decision.PolicyID != authPolicy.PolicyID {
		t.Fatalf("unexpected policy id: %v", decision.PolicyID)
	}
}

func TestDenyAll(t *testing.T) {
	engine := DenyAll{}
	action := &runtime.Action{}
	decision, err := engine.Evaluate(context.Background(), action, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision == nil || decision.Allowed {
		t.Fatal("DenyAll should always return a denied decision")
	}
	if decision.Code != "deny_all" {
		t.Fatalf("unexpected code: %q", decision.Code)
	}
}

var (
	_ PolicyEngine = AllowAll{}
	_ PolicyEngine = DenyAll{}
)
