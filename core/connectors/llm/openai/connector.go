package openai

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/kave-io/kave/core/connectors"
	"github.com/kave-io/kave/core/connectors/llm/shared"
	"github.com/kave-io/kave/core/connectors/runtime"
	"github.com/kave-io/kave/core/pipeline"
	coreruntime "github.com/kave-io/kave/core/runtime"
	"github.com/tidwall/gjson"
)

// APIVersion is the OpenAI API version this connector was built against.
const APIVersion = "v1"

type Connector struct {
	client *Client
}

func NewConnector(client *Client) *Connector {
	return &Connector{client: client}
}

func (c *Connector) Name() string {
	return "openai"
}

func (c *Connector) Intercept(ctx context.Context, action *coreruntime.Action, next connectors.Handler) (*pipeline.Result, error) {
	if action.Connector != "openai" {
		return nil, fmt.Errorf("openai: unexpected connector %q", action.Connector)
	}
	return next(ctx, action)
}

func (c *Connector) Capabilities() connectors.Capabilities {
	return connectors.Capabilities{
		SupportedActions: []coreruntime.ActionType{coreruntime.TypeLLM},
		SupportedMethods: []string{
			"chat.completions",
			"chat.completions.streaming",
			"embeddings",
		},
		CanProxy:      true,
		StreamSupport: true,
		APIVersion:    APIVersion,
	}
}

func (c *Connector) PrepareRequest(call *runtime.LLMCall, credential string) (*runtime.PreparedRequest, error) {
	base, err := url.Parse("https://api.openai.com")
	if err != nil {
		return nil, err
	}
	base.Path = call.UpstreamPath
	base.RawQuery = call.RawQuery

	headers := runtime.CloneHeader(call.Header)
	headers.Del("Authorization")
	headers.Del("Connection")
	headers.Del("Transfer-Encoding")
	headers.Del("Accept-Encoding")
	if credential != "" {
		headers.Set("Authorization", "Bearer "+credential)
	}

	return &runtime.PreparedRequest{
		Method: call.Method,
		URL:    base.String(),
		Header: headers,
		Body:   call.Body,
	}, nil
}

func (c *Connector) ParseResponse(body []byte, contentType string) (*pipeline.Result, error) {
	result := &pipeline.Result{Body: body}

	var input, output int
	var model string

	// Use streaming parser for SSE responses; gjson finds "model" in raw SSE text
	// causing the non-streaming path to think it already parsed the response.
	if strings.Contains(contentType, "event-stream") {
		input = streamUsage(body, "prompt_tokens")
		output = streamUsage(body, "completion_tokens")
		model = streamModel(body)
	} else {
		input = int(gjson.GetBytes(body, "usage.prompt_tokens").Int())
		output = int(gjson.GetBytes(body, "usage.completion_tokens").Int())
		model = gjson.GetBytes(body, "model").String()
	}

	if input != 0 || output != 0 || model != "" {
		result.TokenUsage = &coreruntime.TokenUsage{
			InputTokens:  input,
			OutputTokens: output,
			Model:        model,
		}
	}

	return result, nil
}

func (c *Connector) RequiresAuth() bool {
	return true // OpenAI is a paid provider requiring credentials
}

func streamUsage(body []byte, field string) int {
	for _, line := range shared.SplitSSEDataLines(body) {
		if len(line) == 0 || line == "[DONE]" {
			continue
		}
		val := gjson.Get(line, "usage."+field)
		if val.Exists() {
			return int(val.Int())
		}
	}
	return 0
}

func streamModel(body []byte) string {
	for _, line := range shared.SplitSSEDataLines(body) {
		if len(line) == 0 || line == "[DONE]" {
			continue
		}
		model := gjson.Get(line, "model").String()
		if model != "" {
			return model
		}
	}
	return ""
}
