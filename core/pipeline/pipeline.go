package pipeline

import (
	"context"
	"errors"
	"fmt"

	"github.com/kave-io/kave/core/pkg/ids"
	"github.com/kave-io/kave/core/runtime"
)

type Pipeline struct {
	stages []Stage
}

func New(stages ...Stage) *Pipeline {
	return &Pipeline{stages: stages}
}

// Execute runs all Stage.Before hooks in order, calls handler, then all Stage.After hooks in reverse.
// If any Before returns an error, execution stops — handler and remaining stages do not run.
// After hooks run in reverse regardless of handler error, so cleanup always fires.
// If both the handler and an After hook fail, errors.Join preserves both errors.
func (p *Pipeline) Execute(ctx context.Context, action *runtime.Action, handler Handler) (result *Result, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = fmt.Errorf("pipeline panic: %v", recovered)
		}
	}()

	if action.TraceID == "" {
		traceID, err := ids.TraceID()
		if err != nil {
			return nil, err
		}
		action.TraceID = traceID
	}
	if action.SpanID == "" {
		spanID, err := ids.SpanID()
		if err != nil {
			return nil, err
		}
		action.SpanID = spanID
	}

	traceCtx := runtime.TraceFrom(ctx)
	if traceCtx.SpanID != "" {
		action.ParentID = traceCtx.SpanID
		if action.InvocationRef.ParentID == nil || *action.InvocationRef.ParentID != action.ParentID {
			parentID := action.ParentID
			action.InvocationRef.ParentID = &parentID
		}
	} else {
		action.ParentID = ""
		action.InvocationRef.ParentID = nil
	}

	ctx = runtime.WithTrace(ctx, action.TraceID, action.SpanID)

	for _, ic := range p.stages {
		action, err = ic.Before(ctx, action)
		if err != nil {
			return nil, err
		}
	}

	result, handlerErr := handler(ctx, action)

	var afterErr error
	for i := len(p.stages) - 1; i >= 0; i-- {
		// Every stage must unwind even when another cleanup/settlement stage
		// fails. In particular, observability failure must never suppress usage
		// settlement by an earlier stage in the pipeline.
		afterErr = errors.Join(afterErr, p.stages[i].After(ctx, action, result))
	}
	if afterErr != nil {
		return nil, errors.Join(handlerErr, afterErr)
	}

	return result, handlerErr
}
