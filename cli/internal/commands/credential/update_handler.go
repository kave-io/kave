package credential

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type UpdateInput struct {
}

type UpdateOutput struct {
	Data map[string]any `json:"data"`
}

func RunUpdate(ctx context.Context, in UpdateInput) (*UpdateOutput, error) {
	return nil, output.NotImplemented("credential update")
}
