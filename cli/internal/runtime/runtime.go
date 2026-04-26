package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"

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

	transportOnce sync.Once
	transport     *Transport
	transportErr  error
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

// InjectTransport sets a pre-built Transport, bypassing lazy initialization.
// Intended for tests only.
func (rt *Runtime) InjectTransport(t *Transport) {
	rt.transportOnce.Do(func() {
		rt.transport = t
	})
}

// GetTransport returns the lazily initialized Transport for this runtime.
func (rt *Runtime) GetTransport() (*Transport, error) {
	rt.transportOnce.Do(func() {
		rt.transport, rt.transportErr = newTransport(rt)
	})
	return rt.transport, rt.transportErr
}

// Client returns the HTTP bridge client (backward compat for bridge-only commands).
func (rt *Runtime) Client() *HTTPClient {
	t, err := rt.GetTransport()
	if err != nil {
		// Return a minimal client so callers get a descriptive error on first use.
		return &HTTPClient{BaseURL: "http://127.0.0.1:8080", HTTP: httpClientFromRuntime(rt), SessionKey: ""}
	}
	return t.HTTP()
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
