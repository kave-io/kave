package budget

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
)

type ReportInput struct {
	ProjectID string
	EnvID     string
	PolicyID  string
}

type ReportOutput struct {
	Data *runtimev1.SpendReport `json:"data"`
}

func RunReport(ctx context.Context, in ReportInput) (*ReportOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	t, err := rt.GetTransport()
	if err != nil {
		return nil, err
	}
	svc, err := t.RuntimeSvc()
	if err != nil {
		return nil, err
	}
	req := &runtimev1.GetSpendReportRequest{}
	if in.ProjectID != "" || in.EnvID != "" || in.PolicyID != "" {
		req.Filter = &runtimev1.SpendFilter{ProjectId: in.ProjectID, EnvId: in.EnvID, PolicyId: in.PolicyID}
	}
	resp, err := svc.GetSpendReport(ctx, req)
	if err != nil {
		return nil, err
	}
	return &ReportOutput{Data: resp}, nil
}
