package gateway

import (
	"net/http"
	"testing"

	connruntime "github.com/kave-io/kave/core/connectors/runtime"
)

func TestNewRegistryWithConnectorsAppliesOpenAIBaseURL(t *testing.T) {
	registry := NewRegistryWithConnectors([]ConnectorConfig{
		{
			Name:    "openai",
			Enabled: true,
			Config: map[string]any{
				"base_url": "https://api.gapgpt.app",
			},
		},
	})

	conn, err := registry.ResolveConnector("openai")
	if err != nil {
		t.Fatalf("ResolveConnector: %v", err)
	}

	prepared, err := conn.PrepareRequest(&connruntime.LLMCall{
		Provider:     "openai",
		Method:       http.MethodPost,
		UpstreamPath: "/v1/chat/completions",
		Body:         []byte(`{"model":"gapgpt-qwen-3.6","messages":[]}`),
	}, "sk-test")
	if err != nil {
		t.Fatalf("PrepareRequest: %v", err)
	}

	if got, want := prepared.URL, "https://api.gapgpt.app/v1/chat/completions"; got != want {
		t.Fatalf("prepared URL = %q, want %q", got, want)
	}
}
