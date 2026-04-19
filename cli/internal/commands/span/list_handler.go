package span

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ListInput struct {
}

type ListOutput struct {
	Data map[string]any `json:"data"`
}

func RunList(ctx context.Context, in ListInput) (*ListOutput, error) {
	return nil, output.NotImplemented("span list")
}
