package intercept

import "context"

type ctxKey int

const (
	policyKey ctxKey = iota
	runKey
	tokenUsageKey
)

// TokenUsage carries token counts from an LLM response.
// The proxy handler sets this on the context so cost/trace interceptors can read it.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	CacheRead    int
	CacheWrite   int
	Model        string
}

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

func WithTokenUsage(ctx context.Context, u *TokenUsage) context.Context {
	return context.WithValue(ctx, tokenUsageKey, u)
}

func TokenUsageFrom(ctx context.Context) *TokenUsage {
	u, _ := ctx.Value(tokenUsageKey).(*TokenUsage)
	return u
}
