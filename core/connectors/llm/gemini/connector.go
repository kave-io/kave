package gemini

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

// APIVersion is the Gemini REST API version this connector was built against.
// Appears in the base URL path. Upgrade to "v1" when Google promotes it.
const APIVersion = "v1beta"

type Connector struct {
	client *Client
}

func NewConnector(client *Client) *Connector {
	return &Connector{client: client}
}

func (c *Connector) Name() string { return "gemini" }

func (c *Connector) Intercept(ctx context.Context, action *coreruntime.Action, next connectors.Handler) (*pipeline.Result, error) {
	if action.Connector != "gemini" {
		return nil, fmt.Errorf("gemini: unexpected connector %q", action.Connector)
	}
	return next(ctx, action)
}

func (c *Connector) Capabilities() connectors.Capabilities {
	return connectors.Capabilities{
		SupportedActions: []coreruntime.ActionType{coreruntime.TypeLLM},
		SupportedMethods: []string{"generateContent", "generateContent.streaming"},
		CanProxy:         false,
		StreamSupport:    true,
		APIVersion:       APIVersion,
	}
}

func (c *Connector) PrepareRequest(call *runtime.LLMCall, credential string) (*runtime.PreparedRequest, error) {
	base, err := url.Parse("https://generativelanguage.googleapis.com")
	if err != nil {
		return nil, err
	}
	base.Path = call.UpstreamPath
	query := base.Query()
	if call.RawQuery != "" {
		if parsed, err := url.ParseQuery(call.RawQuery); err == nil {
			for key, values := range parsed {
				for _, value := range values {
					query.Add(key, value)
				}
			}
		}
	}
	if credential != "" {
		query.Set("key", credential)
	}
	base.RawQuery = query.Encode()

	headers := runtime.CloneHeader(call.Header)
	headers.Del("Authorization")
	headers.Del("X-API-Key")
	headers.Del("Connection")
	headers.Del("Transfer-Encoding")
	headers.Del("Accept-Encoding")

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
		InputTokens:  int(gjson.GetBytes(body, "usageMetadata.promptTokenCount").Int()),
		OutputTokens: int(gjson.GetBytes(body, "usageMetadata.candidatesTokenCount").Int()),
		Model:        gjson.GetBytes(body, "modelVersion").String(),
	}
	if usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.Model != "" {
		result.TokenUsage = usage
	}
	return result, nil
}

func (c *Connector) RequiresAuth() bool {
	return true // Google Gemini is a paid provider requiring API credentials
}
