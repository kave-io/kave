package ollama

import (
	"context"
	"fmt"
	"net/url"

	"github.com/kave-io/kave/core/connectors"
	"github.com/kave-io/kave/core/connectors/runtime"
	"github.com/kave-io/kave/core/pipeline"
	coreruntime "github.com/kave-io/kave/core/runtime"
	"github.com/tidwall/gjson"
)

type Connector struct {
	client *Client
}

func NewConnector(client *Client) *Connector {
	return &Connector{client: client}
}

// APIVersion is the Ollama API version this connector was built against.
const APIVersion = "0.6.x"

func (c *Connector) Name() string {
	return "ollama"
}

func (c *Connector) Intercept(ctx context.Context, action *coreruntime.Action, next connectors.Handler) (*pipeline.Result, error) {
	if action.Connector != "ollama" {
		return nil, fmt.Errorf("ollama: unexpected connector %q", action.Connector)
	}
	return next(ctx, action)
}

func (c *Connector) Capabilities() connectors.Capabilities {
	return connectors.Capabilities{
		SupportedActions: []coreruntime.ActionType{coreruntime.TypeLLM},
		SupportedMethods: []string{"chat", "generate", "embed"},
		CanProxy:         false,
		StreamSupport:    true,
		APIVersion:       APIVersion,
	}
}

func (c *Connector) PrepareRequest(call *runtime.LLMCall, credential string) (*runtime.PreparedRequest, error) {
	base, err := url.Parse("http://localhost:11434")
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

func (c *Connector) ParseResponse(body []byte, _ string) (*pipeline.Result, error) {
	result := &pipeline.Result{Body: body}
	usage := &coreruntime.TokenUsage{
		InputTokens:  int(gjson.GetBytes(body, "prompt_eval_count").Int()),
		OutputTokens: int(gjson.GetBytes(body, "eval_count").Int()),
		Model:        gjson.GetBytes(body, "model").String(),
	}
	if usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.Model != "" {
		result.TokenUsage = usage
	}
	return result, nil
}
