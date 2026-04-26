package policy

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type CreateInput struct {
	EnvID       string
	Name        string
	Description string
	Mode        string // "enforce" | "shadow" | ""
}

type CreateOutput struct {
	Data *controlv1.PolicyRecord `json:"data"`
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
	svc, err := t.ControlSvc()
	if err != nil {
		return nil, err
	}
	mode := controlv1.PolicyMode_POLICY_MODE_UNSPECIFIED
	switch in.Mode {
	case "enforce", "ENFORCE":
		mode = controlv1.PolicyMode_POLICY_MODE_ENFORCE
	case "shadow", "SHADOW", "dryrun":
		mode = controlv1.PolicyMode_POLICY_MODE_SHADOW
	}
	rec, err := svc.CreatePolicy(ctx, &controlv1.CreatePolicyRequest{
		EnvId:       in.EnvID,
		Name:        in.Name,
		Description: in.Description,
		Mode:        mode,
	})
	if err != nil {
		return nil, err
	}
	return &CreateOutput{Data: rec}, nil
}
