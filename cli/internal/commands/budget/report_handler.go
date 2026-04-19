package budget

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ReportInput struct {
}

type ReportOutput struct {
	Data map[string]any `json:"data"`
}

func RunReport(ctx context.Context, in ReportInput) (*ReportOutput, error) {
	return nil, output.NotImplemented("budget report")
}
