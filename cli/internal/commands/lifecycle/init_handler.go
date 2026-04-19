package lifecycle

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type InitInput struct {
}

type InitOutput struct {
	Data map[string]any `json:"data"`
}

func RunInit(ctx context.Context, in InitInput) (*InitOutput, error) {
	return nil, output.NotImplemented("lifecycle init")
}
