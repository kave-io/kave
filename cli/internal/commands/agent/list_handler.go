package agent

import (
	"context"
	"fmt"
	"net/url"

	"github.com/kave-io/kave/cli/internal/flags"
	"github.com/kave-io/kave/cli/internal/runtime"
)

type ListInput struct {
	Page flags.PageInput
}

type ListOutput struct {
	Items []map[string]any `json:"items"`
}

func RunList(ctx context.Context, in ListInput) (*ListOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	query := map[string]string{}
	if env := runtime.ActiveEnv(ctx); env != "" {
		query["env_id"] = env
	}
	if in.Page.Limit > 0 {
		query["limit"] = fmt.Sprintf("%d", in.Page.Limit)
	}
	if in.Page.Cursor != "" {
		query["cursor"] = in.Page.Cursor
	}
	var out ListOutput
	if err := rt.Client().Get(ctx, "/api/v1/agents", toValues(query), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func toValues(values map[string]string) url.Values {
	out := make(url.Values, len(values))
	for k, v := range values {
		out.Set(k, v)
	}
	return out
}
