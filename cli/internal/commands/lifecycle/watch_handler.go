package lifecycle

import (
	"context"
	"fmt"
	"time"

	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
	"github.com/kave-io/kave/cli/internal/runtime"
)

type WatchInput struct {
}

type WatchOutput struct {
	Data any `json:"data"`
}

func RunWatch(ctx context.Context, in WatchInput) (*WatchOutput, error) {
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
	streamCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	stream, err := svc.TailTraces(streamCtx, &runtimev1.TailTracesRequest{})
	if err != nil {
		return nil, err
	}
	var frames []any
	for {
		ev, err := stream.Recv()
		if err != nil {
			break
		}
		frames = append(frames, ev)
	}
	return &WatchOutput{Data: map[string]any{"frames": frames}}, nil
}
