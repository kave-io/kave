package ctx

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type CurrentInput struct {
}

type CurrentOutput struct {
	Data map[string]any `json:"data"`
}

func RunCurrent(ctx context.Context, in CurrentInput) (*CurrentOutput, error) {
	return nil, output.NotImplemented("ctx current")
}
