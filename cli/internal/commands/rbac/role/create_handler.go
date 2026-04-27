package role

import (
	"context"
	"fmt"

	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	"github.com/kave-io/kave/cli/internal/runtime"
)

type CreateInput struct {
	Name        string
	Permissions []string
}

type CreateOutput struct {
	Data any `json:"data"`
}

func RunCreate(ctx context.Context, in CreateInput) (*CreateOutput, error) {
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
	resp, err := svc.CreateRole(ctx, &controlv1.CreateRoleRequest{
		Name:        in.Name,
		Permissions: in.Permissions,
	})
	if err != nil {
		return nil, err
	}
	return &CreateOutput{Data: resp}, nil
}
