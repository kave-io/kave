package trace

import (
	"context"
	"fmt"
	"time"

	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
	"github.com/kave-io/kave/cli/internal/runtime"
)

type TailInput struct {
}

type TailOutput struct {
	Data any `json:"data"`
}

func RunTail(ctx context.Context, in TailInput) (*TailOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	t, err := rt.GetTransport()
	if err != nil {
		return nil, err
	}
	svc, err := t.RuntimeSvc()
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stream, err := svc.TailTraces(streamCtx, &runtimev1.TailTracesRequest{})
	if err != nil {
		return nil, err
	}
	var traces []any
	for {
		tr, err := stream.Recv()
		if err != nil {
			break
		}
		traces = append(traces, tr)
	}
	return &TailOutput{Data: traces}, nil
}
