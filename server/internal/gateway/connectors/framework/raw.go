package framework

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kave-io/kave/core/connectors/runtime"
	"github.com/kave-io/kave/core/pkg/ids"
	coreruntime "github.com/kave-io/kave/core/runtime"
	serverllm "github.com/kave-io/kave/server/internal/gateway/connectors/llm"
)

// NewRawLLMFamily builds the provider-only proxy surface used by /v1/* routes.
func NewRawLLMFamily(providers ...serverllm.Providers) LLMFamily {
	llmProviders := serverllm.DefaultProviders()
	if len(providers) > 0 && providers[0] != nil {
		llmProviders = providers[0]
	}

	return LLMFamily{
		Name:      "raw",
		Framework: NewRawConnector(),
		Providers: llmProviders,
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
	if strings.HasPrefix(req.Path, "/backend-api/codex/") {
		return c.parseOpenAICodexRequest(req)
	}

	trimmed := strings.TrimPrefix(req.Path, "/v1/")
	slash := strings.Index(trimmed, "/")
	if slash < 0 {
		return nil, fmt.Errorf("invalid raw path: missing provider upstream path")
	}

	provider := trimmed[:slash]
	upstreamPath := normalizeUpstreamPath(provider, trimmed[slash:])
	upstreamBaseURL := upstreamBaseURL(provider, upstreamPath)
	method := actionMethod(upstreamPath, req.Body)

	inputCopy := make([]byte, len(req.Body))
	copy(inputCopy, req.Body)

	return &runtime.LLMCall{
		Provider:        provider,
		Method:          req.Method,
		UpstreamBaseURL: upstreamBaseURL,
		UpstreamPath:    upstreamPath,
		RawQuery:        req.RawQuery,
		Header:          runtime.CloneHeader(req.Header),
		Body:            req.Body,
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

func (c *RawConnector) parseOpenAICodexRequest(req *runtime.Request) (*runtime.LLMCall, error) {
	inputCopy := make([]byte, len(req.Body))
	copy(inputCopy, req.Body)

	upstreamPath := req.Path
	return &runtime.LLMCall{
		Provider:        "openai",
		Method:          req.Method,
		UpstreamBaseURL: upstreamBaseURL("openai", upstreamPath),
		UpstreamPath:    upstreamPath,
		RawQuery:        req.RawQuery,
		Header:          runtime.CloneHeader(req.Header),
		Body:            req.Body,
		Action: &coreruntime.Action{
			Invocation: coreruntime.Invocation{
				InvocationRef: coreruntime.InvocationRef{
					ID: ids.New("act"),
				},
				InvocationTarget: coreruntime.InvocationTarget{
					Type:      coreruntime.TypeLLM,
					Connector: "openai",
					Method:    actionMethod(upstreamPath, req.Body),
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
	case "openai":
		if strings.HasPrefix(remainder, "/backend-api/codex/") {
			return remainder
		}
		return "/v1" + remainder
	default:
		return remainder
	}
}

func upstreamBaseURL(provider, upstreamPath string) string {
	if provider == "openai" && strings.HasPrefix(upstreamPath, "/backend-api/codex/") {
		if v := strings.TrimSpace(os.Getenv("KAVE_CODEX_CHATGPT_UPSTREAM")); v != "" {
			return v
		}
		return "https://chatgpt.com"
	}
	return ""
}

func actionMethod(upstreamPath string, body []byte) string {
	if strings.HasPrefix(upstreamPath, "/backend-api/codex/") {
		return "codex.responses"
	}
	segments := strings.Split(strings.Trim(upstreamPath, "/"), "/")
	method := strings.Join(segments[1:], ".")
	if len(segments) > 0 && segments[0] != "v1" && segments[0] != "v1beta" {
		method = strings.Join(segments, ".")
	}

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
