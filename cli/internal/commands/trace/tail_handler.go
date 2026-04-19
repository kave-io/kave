package trace

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type TailInput struct {
}

type TailOutput struct {
	Data map[string]any `json:"data"`
}

func RunTail(ctx context.Context, in TailInput) (*TailOutput, error) {
	return nil, output.NotImplemented("trace tail")
}
