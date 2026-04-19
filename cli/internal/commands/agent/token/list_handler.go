package token

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ListInput struct {
	Agent string
}

type ListOutput struct {
	Items []map[string]any `json:"items"`
}

func RunList(ctx context.Context, in ListInput) (*ListOutput, error) {
	return nil, output.NotImplemented("agent token list")
}
