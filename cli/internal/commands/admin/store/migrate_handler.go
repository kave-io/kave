package store

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type MigrateInput struct {
}

type MigrateOutput struct {
	Data any `json:"data"`
}

func RunMigrate(ctx context.Context, in MigrateInput) (*MigrateOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "admin store migrate: not yet implemented", Exit: 1}
}
