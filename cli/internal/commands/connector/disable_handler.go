package connector

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type DisableInput struct {
}

type DisableOutput struct {
	Data map[string]any `json:"data"`
}

func RunDisable(ctx context.Context, in DisableInput) (*DisableOutput, error) {
	return nil, output.NotImplemented("connector disable")
}
