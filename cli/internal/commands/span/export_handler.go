package span

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ExportInput struct {
}

type ExportOutput struct {
	Data any `json:"data"`
}

func RunExport(ctx context.Context, in ExportInput) (*ExportOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "span export: not yet implemented", Exit: 1}
}
