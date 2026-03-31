package anthropic

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/connectors"
	"github.com/kave-io/kave/core/intercept"
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

func (c *Connector) Intercept(ctx context.Context, action *intercept.Action, next connectors.Handler) (*intercept.Result, error) {
	if action.Connector != "anthropic" {
		return nil, fmt.Errorf("anthropic: unexpected connector %q", action.Connector)
	}
	return next(ctx, action)
}

func (c *Connector) Capabilities() connectors.Capabilities {
	return connectors.Capabilities{
		SupportedActions: []intercept.ActionType{intercept.TypeLLM},
		SupportedMethods: []string{"messages", "messages.streaming"},
		CanProxy:         true,
		CanStream:        true,
		APIVersion:       APIVersion,
	}
}
