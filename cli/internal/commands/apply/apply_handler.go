package apply

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ApplyInput struct {
}

type ApplyOutput struct {
	Data map[string]any `json:"data"`
}

func RunApply(ctx context.Context, in ApplyInput) (*ApplyOutput, error) {
	return nil, output.NotImplemented("apply apply")
}
