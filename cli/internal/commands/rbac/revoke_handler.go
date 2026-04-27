package rbac

import (
	"context"
	"fmt"

	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	"github.com/kave-io/kave/cli/internal/runtime"
)

type RevokeInput struct {
	BindingID string
}

type RevokeOutput struct {
	Data any `json:"data"`
}

func RunRevoke(ctx context.Context, in RevokeInput) (*RevokeOutput, error) {
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
	_, err = svc.DeleteBinding(ctx, &controlv1.DeleteBindingRequest{Id: in.BindingID})
	if err != nil {
		return nil, err
	}
	return &RevokeOutput{Data: map[string]any{"status": "ok"}}, nil
}
