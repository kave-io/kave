package budget

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ReportInput struct {
}

type ReportOutput struct {
	Data any `json:"data"`
}

func RunReport(ctx context.Context, in ReportInput) (*ReportOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "budget report is not exposed by the HTTP bridge yet", Exit: 1}
}
