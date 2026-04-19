package fx

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type RefreshInput struct {
}

type RefreshOutput struct {
	Data map[string]any `json:"data"`
}

func RunRefresh(ctx context.Context, in RefreshInput) (*RefreshOutput, error) {
	return nil, output.NotImplemented("fx refresh")
}
