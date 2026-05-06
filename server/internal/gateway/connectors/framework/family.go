package framework

import (
	"github.com/kave-io/kave/core/connectors/runtime"
)

// LLMFamily is the server-side bundle for one framework-facing LLM integration.
// The server owns orchestration, while connectors implement the framework and
// provider contracts without depending on server packages.
type LLMFamily struct {
	Name      string
	Framework runtime.LLMFramework
	Providers map[string]runtime.LLMConnector
}
