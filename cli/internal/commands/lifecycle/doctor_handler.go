package lifecycle

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type DoctorInput struct {
}

type DoctorOutput struct {
	Data any `json:"data"`
}

func RunDoctor(ctx context.Context, in DoctorInput) (*DoctorOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	var out any
	if err := rt.Client().Get(ctx, "/api/v1/doctor", nil, &out); err != nil {
		return nil, err
	}
	return &DoctorOutput{Data: out}, nil
}
