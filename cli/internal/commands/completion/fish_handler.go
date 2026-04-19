package completion

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type FishInput struct {
}

type FishOutput struct {
	Data map[string]any `json:"data"`
}

func RunFish(ctx context.Context, in FishInput) (*FishOutput, error) {
	return nil, output.NotImplemented("completion fish")
}
