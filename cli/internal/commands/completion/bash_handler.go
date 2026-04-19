package completion

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type BashInput struct {
}

type BashOutput struct {
	Data map[string]any `json:"data"`
}

func RunBash(ctx context.Context, in BashInput) (*BashOutput, error) {
	return nil, output.NotImplemented("completion bash")
}
