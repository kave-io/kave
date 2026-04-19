package store

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type MigrateInput struct {
}

type MigrateOutput struct {
	Data map[string]any `json:"data"`
}

func RunMigrate(ctx context.Context, in MigrateInput) (*MigrateOutput, error) {
	return nil, output.NotImplemented("store migrate")
}
