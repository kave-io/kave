package fx

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type RatesInput struct {
}

type RatesOutput struct {
	Data any `json:"data"`
}

func RunRates(ctx context.Context, in RatesInput) (*RatesOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "fx rates is not exposed by the HTTP bridge yet", Exit: 1}
}
