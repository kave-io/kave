package lifecycle

import (
	"context"
	"fmt"
	"time"

	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
	"github.com/kave-io/kave/cli/internal/runtime"
)

type LogsInput struct {
}

type LogsOutput struct {
	Data any `json:"data"`
}

func RunLogs(ctx context.Context, in LogsInput) (*LogsOutput, error) {
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
	stream, err := svc.WatchLogs(streamCtx, &runtimev1.WatchLogsRequest{})
	if err != nil {
		return nil, err
	}
	var lines []any
	for {
		line, err := stream.Recv()
		if err != nil {
			break
		}
		lines = append(lines, line)
	}
	return &LogsOutput{Data: map[string]any{"lines": lines}}, nil
}
