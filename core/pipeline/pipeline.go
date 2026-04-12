package pipeline

import (
	"context"
	"errors"

	"github.com/kave-io/kave/core/runtime"
)

type Pipeline struct {
	interceptors []Interceptor
}

func New(interceptors ...Interceptor) *Pipeline {
	return &Pipeline{interceptors: interceptors}
}

// Execute runs all Before hooks in order, calls handler, then all After hooks in reverse.
// If any Before returns an error, execution stops — handler and remaining interceptors do not run.
// After hooks run in reverse regardless of handler error, so cleanup always fires.
// If both the handler and an After hook fail, errors.Join preserves both errors.
func (p *Pipeline) Execute(ctx context.Context, action *runtime.Action, handler Handler) (*Result, error) {
	var err error

	for _, ic := range p.interceptors {
		action, err = ic.Before(ctx, action)
		if err != nil {
			return nil, err
		}
	}

	result, handlerErr := handler(ctx, action)

	for i := len(p.interceptors) - 1; i >= 0; i-- {
		if err = p.interceptors[i].After(ctx, action, result); err != nil {
			// errors.Join(nil, afterErr) == afterErr, so when handler succeeded
			// behaviour is unchanged. When both fail, both errors are preserved.
			return nil, errors.Join(handlerErr, err)
		}
	}

	return result, handlerErr
}
