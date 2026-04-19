package version

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type VersionInput struct {
}

type VersionOutput struct {
	Data map[string]any `json:"data"`
}

func RunVersion(ctx context.Context, in VersionInput) (*VersionOutput, error) {
	return nil, output.NotImplemented("version version")
}
