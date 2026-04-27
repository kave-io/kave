package events

import (
	"context"
	"fmt"
	"time"

	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
	"github.com/kave-io/kave/cli/internal/runtime"
)

type ListInput struct {
}

type ListOutput struct {
	Data any `json:"data"`
}

func RunList(ctx context.Context, in ListInput) (*ListOutput, error) {
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
	streamCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	stream, err := svc.WatchEvents(streamCtx, &runtimev1.WatchEventsRequest{})
	if err != nil {
		return nil, err
	}
	var events []any
	for {
		ev, err := stream.Recv()
		if err != nil {
			break
		}
		events = append(events, ev)
	}
	return &ListOutput{Data: events}, nil
}
