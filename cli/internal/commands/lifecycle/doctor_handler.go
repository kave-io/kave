package lifecycle

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	"google.golang.org/protobuf/types/known/emptypb"
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
	t, err := rt.GetTransport()
	if err != nil {
		return nil, err
	}
	svc, err := t.DaemonSvc()
	if err != nil {
		return nil, err
	}
	resp, err := svc.Doctor(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	return &DoctorOutput{Data: resp}, nil
}
