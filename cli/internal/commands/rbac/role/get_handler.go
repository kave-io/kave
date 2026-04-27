package role

import (
	"context"
	"fmt"

	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	"github.com/kave-io/kave/cli/internal/runtime"
)

type GetInput struct {
	ID string
}

type GetOutput struct {
	Data any `json:"data"`
}

func RunGet(ctx context.Context, in GetInput) (*GetOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	t, err := rt.GetTransport()
	if err != nil {
		return nil, err
	}
	svc, err := t.RBACSvc()
	if err != nil {
		return nil, err
	}
	resp, err := svc.GetRole(ctx, &controlv1.GetRoleRequest{Id: in.ID})
	if err != nil {
		return nil, err
	}
	return &GetOutput{Data: resp}, nil
}
