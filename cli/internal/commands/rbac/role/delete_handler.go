package role

import (
	"context"
	"fmt"

	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	"github.com/kave-io/kave/cli/internal/runtime"
)

type DeleteInput struct {
	ID string
}

type DeleteOutput struct {
	Data any `json:"data"`
}

func RunDelete(ctx context.Context, in DeleteInput) (*DeleteOutput, error) {
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
	_, err = svc.DeleteRole(ctx, &controlv1.DeleteRoleRequest{Id: in.ID})
	if err != nil {
		return nil, err
	}
	return &DeleteOutput{Data: map[string]any{"status": "ok"}}, nil
}
