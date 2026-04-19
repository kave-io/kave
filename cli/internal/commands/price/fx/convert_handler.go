package fx

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ConvertInput struct {
}

type ConvertOutput struct {
	Data map[string]any `json:"data"`
}

func RunConvert(ctx context.Context, in ConvertInput) (*ConvertOutput, error) {
	return nil, output.NotImplemented("fx convert")
}
