package ollama

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/connectors"
	"github.com/kave-io/kave/core/intercept"
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

func (c *Connector) Intercept(ctx context.Context, action *intercept.Action, next connectors.Handler) (*intercept.Result, error) {
	if action.Connector != "ollama" {
		return nil, fmt.Errorf("ollama: unexpected connector %q", action.Connector)
	}
	return next(ctx, action)
}

func (c *Connector) Capabilities() connectors.Capabilities {
	return connectors.Capabilities{
		SupportedActions: []intercept.ActionType{intercept.TypeLLM},
		SupportedMethods: []string{"chat", "generate", "embed"},
		CanProxy:         false,
		CanStream:        true,
		APIVersion:       APIVersion,
	}
}
