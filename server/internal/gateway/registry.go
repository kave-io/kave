package gateway

import (
	"fmt"

	"github.com/kave-io/kave/core/connectors/runtime"
	serverframework "github.com/kave-io/kave/server/internal/gateway/connectors/framework"
)

// Registry resolves framework names to their LLM family bundles.
type Registry struct {
	frameworks map[string]serverframework.LLMFamily
}

// NewRegistry builds the default framework registry.
func NewRegistry() *Registry {
	return &Registry{
		frameworks: map[string]serverframework.LLMFamily{
			"raw":         serverframework.NewRawLLMFamily(),
			"claude-code": serverframework.NewClaudeCodeLLMFamily(),
		},
	}
}

// Register adds or replaces a framework family.
func (r *Registry) Register(family serverframework.LLMFamily) {
	if r.frameworks == nil {
		r.frameworks = make(map[string]serverframework.LLMFamily)
	}
	r.frameworks[family.Name] = family
}

// ResolveConnector returns the LLMConnector for a given provider name (e.g. "openai", "ollama").
// It searches across all registered framework families.
func (r *Registry) ResolveConnector(provider string) (runtime.LLMConnector, error) {
	for _, family := range r.frameworks {
		if conn, ok := family.Providers[provider]; ok {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("no connector for provider %q", provider)
}

// Resolve returns the family for the given framework name.
func (r *Registry) Resolve(framework string) (serverframework.LLMFamily, error) {
	if r == nil || len(r.frameworks) == 0 {
		return serverframework.LLMFamily{}, fmt.Errorf("framework registry is empty")
	}
	family, ok := r.frameworks[framework]
	if !ok {
		return serverframework.LLMFamily{}, fmt.Errorf("unknown framework: %q", framework)
	}
	return family, nil
}
