package price

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ImportInput struct {
}

type ImportOutput struct {
	Data map[string]any `json:"data"`
}

func RunImport(ctx context.Context, in ImportInput) (*ImportOutput, error) {
	return nil, output.NotImplemented("price import")
}
