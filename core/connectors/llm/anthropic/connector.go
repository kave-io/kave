package anthropic

import (
	"context"
	"fmt"
	"net/url"

	"github.com/kave-io/kave/core/connectors"
	"github.com/kave-io/kave/core/connectors/llm/shared"
	"github.com/kave-io/kave/core/connectors/runtime"
	"github.com/kave-io/kave/core/pipeline"
	coreruntime "github.com/kave-io/kave/core/runtime"
	"github.com/tidwall/gjson"
)

// APIVersion is the Anthropic API version this connector was built against.
// Sent as the anthropic-version header on every request.
const APIVersion = "2023-06-01"

type Connector struct {
	client *Client
}

func NewConnector(client *Client) *Connector {
	return &Connector{client: client}
}

func (c *Connector) Name() string { return "anthropic" }

func (c *Connector) Intercept(ctx context.Context, action *coreruntime.Action, next connectors.Handler) (*pipeline.Result, error) {
	if action.Connector != "anthropic" {
		return nil, fmt.Errorf("anthropic: unexpected connector %q", action.Connector)
	}
	return next(ctx, action)
}

func (c *Connector) Capabilities() connectors.Capabilities {
	return connectors.Capabilities{
		SupportedActions: []coreruntime.ActionType{coreruntime.TypeLLM},
		SupportedMethods: []string{"messages", "messages.streaming"},
		CanProxy:         true,
		StreamSupport:    true,
		APIVersion:       APIVersion,
	}
}

func (c *Connector) PrepareRequest(call *runtime.LLMCall, credential string) (*runtime.PreparedRequest, error) {
	base, err := url.Parse("https://api.anthropic.com")
	if err != nil {
		return nil, err
	}
	base.Path = call.UpstreamPath
	base.RawQuery = call.RawQuery

	headers := runtime.CloneHeader(call.Header)
	headers.Del("Authorization")
	headers.Del("X-API-Key")
	headers.Del("Connection")
	headers.Del("Transfer-Encoding")
	headers.Del("Accept-Encoding")
	if credential != "" {
		headers.Set("X-API-Key", credential)
	}
	headers.Set("Anthropic-Version", APIVersion)

	return &runtime.PreparedRequest{
		Method: call.Method,
		URL:    base.String(),
		Header: headers,
		Body:   call.Body,
	}, nil
}

func (c *Connector) ParseResponse(body []byte, _ string) (*pipeline.Result, error) {
	result := &pipeline.Result{Body: body}

	usage := &coreruntime.TokenUsage{
		InputTokens:  int(gjson.GetBytes(body, "usage.input_tokens").Int()),
		OutputTokens: int(gjson.GetBytes(body, "usage.output_tokens").Int()),
		CacheRead:    int(gjson.GetBytes(body, "usage.cache_read_input_tokens").Int()),
		CacheWrite:   int(gjson.GetBytes(body, "usage.cache_creation_input_tokens").Int()),
		Model:        gjson.GetBytes(body, "model").String(),
	}

	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.Model == "" && usage.CacheRead == 0 && usage.CacheWrite == 0 {
		for _, line := range shared.SplitSSEDataLines(body) {
			if line == "" || line == "[DONE]" {
				continue
			}
			if usage.InputTokens == 0 {
				usage.InputTokens = int(gjson.Get(line, "message.usage.input_tokens").Int())
			}
			if usage.CacheRead == 0 {
				usage.CacheRead = int(gjson.Get(line, "message.usage.cache_read_input_tokens").Int())
			}
			if usage.CacheWrite == 0 {
				usage.CacheWrite = int(gjson.Get(line, "message.usage.cache_creation_input_tokens").Int())
			}
			if usage.OutputTokens == 0 {
				usage.OutputTokens = int(gjson.Get(line, "usage.output_tokens").Int())
			}
			if usage.Model == "" {
				usage.Model = gjson.Get(line, "message.model").String()
			}
		}
	}

	if usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.Model != "" || usage.CacheRead != 0 || usage.CacheWrite != 0 {
		result.TokenUsage = usage
	}

	return result, nil
}

func (c *Connector) RequiresAuth() bool {
	return true // Anthropic is a paid provider requiring credentials
}
