package policy

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type ExportInput struct {
	ID string
}

type ExportOutput struct {
	YAML string `json:"yaml"`
}

func RunExport(ctx context.Context, in ExportInput) (*ExportOutput, error) {
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
	resp, err := svc.ExportPolicy(ctx, &controlv1.ExportPolicyRequest{Id: in.ID})
	if err != nil {
		return nil, err
	}
	return &ExportOutput{YAML: resp.GetYaml()}, nil
}
