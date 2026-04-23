package llm

import "github.com/kave-io/kave/core/connectors/runtime"

// Providers is the server-side bundle of supported LLM provider connectors.
// The implementations live in /connectors/llm, while the server selects which
// provider set a framework can expose.
type Providers map[string]runtime.LLMConnector

func DefaultProviders() Providers {
	return BuildProviders(nil)
}
