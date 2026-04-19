package fx

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type RatesInput struct {
}

type RatesOutput struct {
	Data map[string]any `json:"data"`
}

func RunRates(ctx context.Context, in RatesInput) (*RatesOutput, error) {
	return nil, output.NotImplemented("fx rates")
}
