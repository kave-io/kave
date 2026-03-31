package auth

import (
	"context"
	"testing"

	"github.com/kave-io/kave/core/intercept"
)

func TestAllowAll(t *testing.T) {
	engine := AllowAll{}
	policy := &intercept.Policy{
		AllowedTypes:      []string{"llm"},
		AllowedConnectors: []string{"openai"},
		AllowedMethods:    []string{"chat.completions"},
	}

	ok, err := engine.Allowed(context.Background(), intercept.TypeLLM, "openai", "chat.completions", policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("AllowAll should always return true")
	}
}

func TestDenyAll(t *testing.T) {
	engine := DenyAll{}
	ok, err := engine.Allowed(context.Background(), intercept.TypeLLM, "openai", "chat.completions", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("DenyAll should always return false")
	}
}

// Verify both implement the interface at compile time.
var (
	_ PolicyEngine = AllowAll{}
	_ PolicyEngine = DenyAll{}
)
