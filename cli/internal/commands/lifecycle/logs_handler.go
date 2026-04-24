package lifecycle

import (
	"bufio"
	"context"
	"fmt"
	"time"

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
	streamCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	rc, err := rt.Client().Stream(streamCtx, "/api/v1/logs/tail", nil)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	scanner := bufio.NewScanner(rc)
	if !scanner.Scan() {
		return &LogsOutput{Data: map[string]any{"status": "ok"}}, scanner.Err()
	}
	return &LogsOutput{Data: map[string]any{"frame": scanner.Text()}}, nil
}
