package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/kave-io/kave/cli/internal/config"
	"github.com/kave-io/kave/cli/internal/output"
)

type contextKey struct{}

var ErrNotImplemented = errors.New("not implemented")

type Invocation struct {
	CommandPath string
	Args        []string
	Flags       map[string]any
	Interactive bool
}

type Dispatcher interface {
	Dispatch(context.Context, Invocation) error
}

type Services struct {
	Dispatcher Dispatcher
}

type Runtime struct {
	Resolution *config.Resolution
	Output     output.Format
	Services   Services
}

func New(resolution *config.Resolution, outputFormat output.Format) *Runtime {
	return &Runtime{
		Resolution: resolution,
		Output:     outputFormat,
		Services: Services{
			Dispatcher: NotImplementedDispatcher{},
		},
	}
}

func WithContext(ctx context.Context, rt *Runtime) context.Context {
	return context.WithValue(ctx, contextKey{}, rt)
}

func FromContext(ctx context.Context) (*Runtime, bool) {
	rt, ok := ctx.Value(contextKey{}).(*Runtime)
	return rt, ok
}

type NotImplementedDispatcher struct{}

func (NotImplementedDispatcher) Dispatch(_ context.Context, invocation Invocation) error {
	return &output.CommandError{
		Code:    "command.unavailable",
		Message: fmt.Sprintf("%s is not exposed by the HTTP bridge yet", invocation.CommandPath),
		Exit:    1,
	}
}

func MustDispatch(rt *Runtime, ctx context.Context, invocation Invocation) error {
	if rt == nil || rt.Services.Dispatcher == nil {
		return fmt.Errorf("runtime dispatcher missing")
	}
	return rt.Services.Dispatcher.Dispatch(ctx, invocation)
}

func ActiveConfig(ctx context.Context) *config.ContextConfig {
	rt, ok := FromContext(ctx)
	if !ok || rt == nil || rt.Resolution == nil {
		return nil
	}
	return rt.Resolution.ActiveContext()
}

func ActiveServer(ctx context.Context) string {
	rt, ok := FromContext(ctx)
	if !ok || rt == nil || rt.Resolution == nil {
		return ""
	}
	return rt.Resolution.ActiveServer()
}

func ActiveProject(ctx context.Context) string {
	if cfg := ActiveConfig(ctx); cfg != nil {
		return cfg.Project
	}
	return ""
}

func ActiveEnv(ctx context.Context) string {
	if cfg := ActiveConfig(ctx); cfg != nil {
		return cfg.Env
	}
	return ""
}
