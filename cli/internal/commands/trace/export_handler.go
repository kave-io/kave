package trace

import (
	"context"
	"fmt"

	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
	"github.com/kave-io/kave/cli/internal/runtime"
)

type ExportInput struct {
	TraceID string
}

type ExportOutput struct {
	Data any `json:"data"`
}

func RunExport(ctx context.Context, in ExportInput) (*ExportOutput, error) {
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
	resp, err := svc.ExportTrace(ctx, &runtimev1.ExportTraceRequest{TraceId: in.TraceID})
	if err != nil {
		return nil, err
	}
	return &ExportOutput{Data: map[string]any{"content_type": resp.ContentType, "data": string(resp.Data)}}, nil
}
