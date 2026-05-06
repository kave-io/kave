package llm

import (
	"github.com/kave-io/kave/core/connectors/llm/openai"
	"github.com/kave-io/kave/core/connectors/runtime"
)

// Factory builds a provider connector instance.
type Factory func() runtime.LLMConnector

// ProviderFactories lists the built-in provider connector constructors.
var ProviderFactories = map[string]Factory{
	"openai": func() runtime.LLMConnector { return openai.NewConnector(nil) },
}

// BuildProviders returns the built-in providers, optionally filtered to the
// requested names. Unknown names are skipped so config can enable a subset.
func BuildProviders(enabled []string) Providers {
	out := make(Providers)
	if len(enabled) == 0 {
		for name, factory := range ProviderFactories {
			out[name] = factory()
		}
		return out
	}
	for _, name := range enabled {
		if factory, ok := ProviderFactories[name]; ok {
			out[name] = factory()
		}
	}
	return out
}
