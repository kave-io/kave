package intercept

import "context"

type ctxKey int

const (
	policyKey ctxKey = iota
	runKey
)

func WithPolicy(ctx context.Context, p *Policy) context.Context {
	return context.WithValue(ctx, policyKey, p)
}

func PolicyFrom(ctx context.Context) *Policy {
	p, _ := ctx.Value(policyKey).(*Policy)
	return p
}

func WithRun(ctx context.Context, r *Run) context.Context {
	return context.WithValue(ctx, runKey, r)
}

func RunFrom(ctx context.Context) *Run {
	r, _ := ctx.Value(runKey).(*Run)
	return r
}
