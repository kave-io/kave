package rbac

import (
	"context"
	"fmt"

	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	"github.com/kave-io/kave/cli/internal/runtime"
)

type GrantInput struct {
	RoleID  string
	Subject string
	Scope   string
}

type GrantOutput struct {
	Data any `json:"data"`
}

func RunGrant(ctx context.Context, in GrantInput) (*GrantOutput, error) {
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
	resp, err := svc.CreateBinding(ctx, &controlv1.CreateBindingRequest{
		RoleId:  in.RoleID,
		Subject: in.Subject,
		Scope:   in.Scope,
	})
	if err != nil {
		return nil, err
	}
	return &GrantOutput{Data: resp}, nil
}
