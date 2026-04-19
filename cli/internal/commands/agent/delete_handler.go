package agent

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type DeleteInput struct {
	ID   string
	Hard bool
}

type DeleteOutput struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

func RunDelete(ctx context.Context, in DeleteInput) (*DeleteOutput, error) {
	return nil, output.NotImplemented("agent delete")
}
