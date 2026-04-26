package policy

import (
	"context"
	"fmt"
	"os"

	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type ValidateInput struct {
	YAML string
	File string
}

type ValidateOutput struct {
	OK     bool     `json:"ok"`
	Issues []string `json:"issues,omitempty"`
}

func RunValidate(ctx context.Context, in ValidateInput) (*ValidateOutput, error) {
	yaml := in.YAML
	if yaml == "" && in.File != "" {
		b, err := os.ReadFile(in.File)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", in.File, err)
		}
		yaml = string(b)
	}
	if yaml == "" {
		return nil, fmt.Errorf("policy yaml required (--yaml or --file)")
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
	resp, err := svc.ValidatePolicy(ctx, &controlv1.ValidatePolicyRequest{Yaml: yaml})
	if err != nil {
		return nil, err
	}
	return &ValidateOutput{OK: resp.GetOk(), Issues: resp.GetIssues()}, nil
}
