package role

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type DeleteInput struct {
}

type DeleteOutput struct {
	Data map[string]any `json:"data"`
}

func RunDelete(ctx context.Context, in DeleteInput) (*DeleteOutput, error) {
	return nil, output.NotImplemented("role delete")
}
