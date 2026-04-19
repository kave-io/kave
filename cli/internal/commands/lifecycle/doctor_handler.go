package lifecycle

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type DoctorInput struct {
}

type DoctorOutput struct {
	Data map[string]any `json:"data"`
}

func RunDoctor(ctx context.Context, in DoctorInput) (*DoctorOutput, error) {
	return nil, output.NotImplemented("lifecycle doctor")
}
