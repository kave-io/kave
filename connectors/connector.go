package connectors

import (
	"context"

	"github.com/kave-io/kave/core/intercept"
)

// Connector is what every framework adapter, LLM proxy, and tool connector implements.
// One interface, everything plugs in.
type Connector interface {
	// Name returns the connector identifier: "openai", "stripe", "langchain"
	Name() string

	// Intercept wraps the connector's execution with Kave's pipeline.
	// The connector calls next() to actually execute the action,
	// then Kave's pipeline runs Before/After hooks around it.
	Intercept(ctx context.Context, action *intercept.Action, next Handler) (*intercept.Result, error)

	// Capabilities declares what this connector can do.
	Capabilities() Capabilities
}

// Handler is the function that executes an action within a connector.
type Handler func(ctx context.Context, action *intercept.Action) (*intercept.Result, error)

// Capabilities describes what a connector supports.
type Capabilities struct {
	// SupportedActions lists which action types this connector handles.
	SupportedActions []intercept.ActionType

	// SupportedMethods lists which specific methods/operations are available.
	SupportedMethods []string

	// CanProxy indicates if this connector can work as an HTTP proxy.
	CanProxy bool

	// CanStream indicates if this connector supports streaming responses.
	CanStream bool

	// APIVersion is the provider API version this connector was built against.
	// Used to detect drift when the upstream API changes.
	APIVersion string
}
