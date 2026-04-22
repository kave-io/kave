package framework

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kave-io/kave/connectors/runtime"
	"github.com/kave-io/kave/core/pkg/ids"
	coreruntime "github.com/kave-io/kave/core/runtime"
	serverllm "github.com/kave-io/kave/server/internal/connectors/llm"
)

// NewRawLLMFamily builds the provider-only proxy surface used by /v1/* routes.
func NewRawLLMFamily() LLMFamily {
	return LLMFamily{
		Name:      "raw",
		Framework: NewRawConnector(),
		Providers: serverllm.DefaultProviders(),
	}
}

// RawConnector parses direct provider URLs without framework-specific prefixes.
type RawConnector struct{}

func NewRawConnector() *RawConnector {
	return &RawConnector{}
}

func (c *RawConnector) Name() string {
	return "raw"
}

func (c *RawConnector) ParseLLMRequest(req *runtime.Request) (*runtime.LLMCall, error) {
	trimmed := strings.TrimPrefix(req.Path, "/v1/")
	slash := strings.Index(trimmed, "/")
	if slash < 0 {
		return nil, fmt.Errorf("invalid raw path: missing provider upstream path")
	}

	provider := trimmed[:slash]
	upstreamPath := normalizeUpstreamPath(provider, trimmed[slash:])
	method := actionMethod(upstreamPath, req.Body)

	inputCopy := make([]byte, len(req.Body))
	copy(inputCopy, req.Body)

	return &runtime.LLMCall{
		Provider:     provider,
		Method:       req.Method,
		UpstreamPath: upstreamPath,
		RawQuery:     req.RawQuery,
		Header:       runtime.CloneHeader(req.Header),
		Body:         req.Body,
		Action: &coreruntime.Action{
			Invocation: coreruntime.Invocation{
				InvocationRef: coreruntime.InvocationRef{
					ID: ids.New("act"),
				},
				InvocationTarget: coreruntime.InvocationTarget{
					Type:      coreruntime.TypeLLM,
					Connector: provider,
					Method:    method,
				},
				InvocationData: coreruntime.InvocationData{
					Input: &inputCopy,
				},
			},
			Status: coreruntime.StatusPending,
		},
	}, nil
}

func normalizeUpstreamPath(provider, remainder string) string {
	remainder = "/" + strings.TrimPrefix(remainder, "/")
	if strings.HasPrefix(remainder, "/v1/") || strings.HasPrefix(remainder, "/v1beta/") {
		return remainder
	}
	switch provider {
	case "openai", "anthropic":
		return "/v1" + remainder
	case "google", "gemini":
		return "/v1beta" + remainder
	default:
		return remainder
	}
}

func actionMethod(upstreamPath string, body []byte) string {
	segments := strings.Split(strings.Trim(upstreamPath, "/"), "/")
	method := segments[len(segments)-1]

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return method
	}

	stream, ok := payload["stream"].(bool)
	if ok && stream {
		return method + ".streaming"
	}

	return method
}
