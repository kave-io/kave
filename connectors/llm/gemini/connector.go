package gemini

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/connectors"
	"github.com/kave-io/kave/core/intercept"
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

func (c *Connector) Intercept(ctx context.Context, action *intercept.Action, next connectors.Handler) (*intercept.Result, error) {
	if action.Connector != "gemini" {
		return nil, fmt.Errorf("gemini: unexpected connector %q", action.Connector)
	}
	return next(ctx, action)
}

func (c *Connector) Capabilities() connectors.Capabilities {
	return connectors.Capabilities{
		SupportedActions: []intercept.ActionType{intercept.TypeLLM},
		SupportedMethods: []string{"generateContent", "generateContent.streaming"},
		CanProxy:         false,
		CanStream:        true,
		APIVersion:       APIVersion,
	}
}
