package pipeline

import (
	"context"

	"github.com/kave-io/kave/core/runtime"
)

// Handler executes the actual work — forwarding the request, calling the tool, etc.
// The pipeline calls it between Before and After hooks.
type Handler func(ctx context.Context, action *runtime.Action) (*Result, error)

// Interceptor wraps an action execution. Before can block by returning a non-nil error.
// After runs in reverse order after the handler completes.
type Interceptor interface {
	Before(ctx context.Context, action *runtime.Action) (*runtime.Action, error)
	After(ctx context.Context, action *runtime.Action, result *Result) error
	Name() string
}
