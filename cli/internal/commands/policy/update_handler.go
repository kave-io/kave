package policy

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type UpdateInput struct {
	ID                string
	Description       *string
	AllowedTypes      []string
	AllowedConnectors []string
	AllowedMethods    []string
}

type UpdateOutput struct {
	Data *controlv1.PolicyRecord `json:"data"`
}

func RunUpdate(ctx context.Context, in UpdateInput) (*UpdateOutput, error) {
	if in.ID == "" {
		return nil, fmt.Errorf("policy id required")
	}
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	t, err := rt.GetTransport()
	if err != nil {
		return nil, err
	}
	svc, err := t.ControlSvc()
	if err != nil {
		return nil, err
	}
	upd := &controlv1.PolicyUpdate{
		Description:       in.Description,
		AllowedTypes:      in.AllowedTypes,
		AllowedConnectors: in.AllowedConnectors,
		AllowedMethods:    in.AllowedMethods,
	}
	rec, err := svc.UpdatePolicy(ctx, &controlv1.UpdatePolicyRequest{Id: in.ID, Update: upd})
	if err != nil {
		return nil, err
	}
	return &UpdateOutput{Data: rec}, nil
}
