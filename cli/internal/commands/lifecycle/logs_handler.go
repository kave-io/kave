package lifecycle

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type LogsInput struct {
}

type LogsOutput struct {
	Data map[string]any `json:"data"`
}

func RunLogs(ctx context.Context, in LogsInput) (*LogsOutput, error) {
	return nil, output.NotImplemented("lifecycle logs")
}
