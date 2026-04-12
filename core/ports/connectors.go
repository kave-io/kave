package ports

import (
	"context"

	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/runtime"
)

// Kind classifies the broad shape of a connector.
type Kind string

const (
	KindFramework Kind = "framework"
	KindProtocol  Kind = "protocol"
	KindProvider  Kind = "provider"
	KindImport    Kind = "import"
)

// Capabilities describes what a connector can do.
type Capabilities struct {
	Kind             Kind
	SupportedActions []runtime.ActionType
	SupportedMethods []string
	CanProxy         bool
	StreamSupport    bool
	APIVersion       string
}

// Descriptor provides connector metadata and capability discovery.
type Descriptor interface {
	Name() string
	Capabilities() Capabilities
}

// Runtime wraps execution for a connector.
type Runtime interface {
	Intercept(ctx context.Context, action *runtime.Action, next Handler) (*pipeline.Result, error)
}

// Connector composes metadata and runtime wrapping.
type Connector interface {
	Descriptor
	Runtime
}

// Handler executes an action within a connector.
type Handler = pipeline.Handler
