package framework

import (
	"github.com/kave-io/kave/connectors/frameworks/claudecode"
	"github.com/kave-io/kave/connectors/runtime"
	serverllm "github.com/kave-io/kave/server/internal/connectors/llm"
)

// LLMFamily is the server-side bundle for one framework-facing LLM integration.
// The server owns orchestration, while connectors implement the framework and
// provider contracts without depending on server packages.
type LLMFamily struct {
	Name      string
	Framework runtime.LLMFramework
	Providers map[string]runtime.LLMConnector
}

func NewClaudeCodeLLMFamily() LLMFamily {
	return LLMFamily{
		Name:      "claude-code",
		Framework: claudecode.NewConnector(),
		Providers: serverllm.DefaultProviders(),
	}
}
