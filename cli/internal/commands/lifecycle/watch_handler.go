package lifecycle

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type WatchInput struct {
}

type WatchOutput struct {
	Data map[string]any `json:"data"`
}

func RunWatch(ctx context.Context, in WatchInput) (*WatchOutput, error) {
	return nil, output.NotImplemented("lifecycle watch")
}
