package span

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/flags"
	"github.com/kave-io/kave/cli/internal/runtime"
	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
)

type ListInput struct {
	Page  flags.PageInput
	RunID string
}

type ListOutput struct {
	Items []*runtimev1.SpanRow `json:"items"`
}

func RunList(ctx context.Context, in ListInput) (*ListOutput, error) {
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
	filter := &runtimev1.SpanFilter{RunId: in.RunID}
	resp, err := svc.QuerySpans(ctx, &runtimev1.QuerySpansRequest{Filter: filter})
	if err != nil {
		return nil, err
	}
	return &ListOutput{Items: resp.Spans}, nil
}
