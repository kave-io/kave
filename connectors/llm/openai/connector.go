package openai

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/connectors"
	"github.com/kave-io/kave/core/intercept"
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

func (c *Connector) Intercept(ctx context.Context, action *intercept.Action, next connectors.Handler) (*intercept.Result, error) {
	if action.Connector != "openai" {
		return nil, fmt.Errorf("openai: unexpected connector %q", action.Connector)
	}
	return next(ctx, action)
}

func (c *Connector) Capabilities() connectors.Capabilities {
	return connectors.Capabilities{
		SupportedActions: []intercept.ActionType{
			intercept.TypeLLM,
		},
		SupportedMethods: []string{
			"chat.completions",
			"chat.completions.streaming",
			"embeddings",
		},
		CanProxy:   true,
		CanStream:  true,
		APIVersion: APIVersion,
	}
}
