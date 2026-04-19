package ctx

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type UseInput struct {
}

type UseOutput struct {
	Data map[string]any `json:"data"`
}

func RunUse(ctx context.Context, in UseInput) (*UseOutput, error) {
	return nil, output.NotImplemented("ctx use")
}
