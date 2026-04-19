package connector

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type EnableInput struct {
}

type EnableOutput struct {
	Data map[string]any `json:"data"`
}

func RunEnable(ctx context.Context, in EnableInput) (*EnableOutput, error) {
	return nil, output.NotImplemented("connector enable")
}
