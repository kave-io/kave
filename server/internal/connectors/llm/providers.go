package llm

import (
	"github.com/kave-io/kave/connectors/llm/anthropic"
	"github.com/kave-io/kave/connectors/llm/gemini"
	"github.com/kave-io/kave/connectors/llm/ollama"
	"github.com/kave-io/kave/connectors/llm/openai"
	"github.com/kave-io/kave/connectors/runtime"
)

// Providers is the server-side bundle of supported LLM provider connectors.
// The implementations live in /connectors/llm, while the server selects which
// provider set a framework can expose.
type Providers map[string]runtime.LLMConnector

func DefaultProviders() Providers {
	return Providers{
		"openai":    openai.NewConnector(nil),
		"anthropic": anthropic.NewConnector(nil),
		"gemini":    gemini.NewConnector(nil),
		"ollama":    ollama.NewConnector(nil),
	}
}
