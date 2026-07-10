package llm

import (
	"github.com/kave-io/kave/core/connectors/llm/openai"
	"github.com/kave-io/kave/core/connectors/runtime"
)

// ProviderConfig carries provider-specific gateway configuration.
type ProviderConfig struct {
	BaseURL string
}

// Factory builds a provider connector instance.
type Factory func(ProviderConfig) runtime.LLMConnector

// ProviderFactories lists the built-in provider connector constructors.
var ProviderFactories = map[string]Factory{
	"openai": func(cfg ProviderConfig) runtime.LLMConnector {
		return openai.NewConnector(&openai.Config{BaseURL: cfg.BaseURL})
	},
}

// BuildProviders returns the built-in providers, optionally filtered to the
// requested names. Unknown names are skipped so config can enable a subset.
func BuildProviders(enabled []string, configs ...map[string]ProviderConfig) Providers {
	providerConfigs := map[string]ProviderConfig{}
	if len(configs) > 0 && configs[0] != nil {
		providerConfigs = configs[0]
	}

	out := make(Providers)
	if len(enabled) == 0 {
		for name, factory := range ProviderFactories {
			out[name] = factory(providerConfigs[name])
		}
		return out
	}
	for _, name := range enabled {
		if factory, ok := ProviderFactories[name]; ok {
			out[name] = factory(providerConfigs[name])
		}
	}
	return out
}
