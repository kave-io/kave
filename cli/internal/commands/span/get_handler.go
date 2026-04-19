package span

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type GetInput struct {
}

type GetOutput struct {
	Data map[string]any `json:"data"`
}

func RunGet(ctx context.Context, in GetInput) (*GetOutput, error) {
	return nil, output.NotImplemented("span get")
}
