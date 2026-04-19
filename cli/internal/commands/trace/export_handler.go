package trace

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ExportInput struct {
}

type ExportOutput struct {
	Data map[string]any `json:"data"`
}

func RunExport(ctx context.Context, in ExportInput) (*ExportOutput, error) {
	return nil, output.NotImplemented("trace export")
}
