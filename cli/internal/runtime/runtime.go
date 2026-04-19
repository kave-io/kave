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
	return output.NotImplemented(invocation.CommandPath)
}

func MustDispatch(rt *Runtime, ctx context.Context, invocation Invocation) error {
	if rt == nil || rt.Services.Dispatcher == nil {
		return fmt.Errorf("runtime dispatcher missing")
	}
	return rt.Services.Dispatcher.Dispatch(ctx, invocation)
}
