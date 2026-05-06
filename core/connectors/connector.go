package connectors

import (
	"context"

	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/runtime"
)

// Kind classifies the broad shape of a connector.
type Kind string

const (
	KindInbound  Kind = "inbound"
	KindLLM      Kind = "llm"
	KindTool     Kind = "tool"
	KindProtocol Kind = "protocol"
	KindImport   Kind = "import"

	// Compatibility aliases for older call sites while the connector tree moves
	// to the intercept/observe boundary terminology.
	KindFramework Kind = KindInbound
	KindProvider  Kind = KindLLM
)

// Capabilities describes what a connector can do.
type Capabilities struct {
	Kind             Kind
	SupportedActions []runtime.ActionType
	SupportedMethods []string
	SupportedRoutes  []string
	RequiresAuth     bool
	CanProxy         bool
	StreamSupport    bool
	APIVersion       string
}

// StaticDescriptor is the registry/read-model shape used by the gateway, CLI,
// dashboard, and architecture linter.
type StaticDescriptor struct {
	ID   string
	Caps Capabilities
}

func (d StaticDescriptor) Name() string { return d.ID }

func (d StaticDescriptor) Capabilities() Capabilities { return d.Caps }

// Descriptor provides connector metadata and capability discovery.
type Descriptor interface {
	Name() string
	Capabilities() Capabilities
}

// Handler executes an action within a connector.
type Handler = pipeline.Handler

// Runtime wraps execution for a connector.
type Runtime interface {
	Intercept(ctx context.Context, action *runtime.Action, next Handler) (*pipeline.Result, error)
}

// Connector composes metadata and runtime wrapping.
type Connector interface {
	Descriptor
	Runtime
}
