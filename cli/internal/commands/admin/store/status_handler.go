package store

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type StatusInput struct {
}

type StatusOutput struct {
	Data map[string]any `json:"data"`
}

func RunStatus(ctx context.Context, in StatusInput) (*StatusOutput, error) {
	return nil, output.NotImplemented("store status")
}
