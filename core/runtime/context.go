package runtime

import (
	"context"

	"github.com/kave-io/kave/core/runtime/policy"
)

type ctxKey int

const (
	policyKey ctxKey = iota
	runKey
	tokenUsageKey
	usageKey
	validationMetaKey
)

// TokenUsage carries token counts from an LLM response.
// The proxy handler sets this on the context so cost/trace interceptors can read it.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	CacheRead    int
	CacheWrite   int
	Reasoning    int  // output reasoning tokens (OpenAI o1/o3, Claude thinking)
	AudioInput   int  // audio input tokens (OpenAI audio models)
	AudioOutput  int  // audio output tokens (OpenAI audio models)
	ImageUnits   int  // provider-agnostic media unit count (OpenAI image_tokens, etc.)
	Model        string
}

// Usage carries all metered dimensions for one action.
// Tokens is nil for non-LLM actions.
type Usage struct {
	Tokens         *TokenUsage // nil for non-LLM actions
	RequestCount   int         // number of external requests issued
	ComputeMs      int64       // server-side compute time (provider-reported, e.g. Replicate)
	StorageBytes   int64       // bytes stored (vector DBs, file writes)
	BandwidthBytes int64       // bytes transferred (outbound API calls)
}

func WithPolicy(ctx context.Context, p *policy.Policy) context.Context {
	return context.WithValue(ctx, policyKey, p)
}

func PolicyFrom(ctx context.Context) *policy.Policy {
	p, _ := ctx.Value(policyKey).(*policy.Policy)
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

func WithUsage(ctx context.Context, u *Usage) context.Context {
	return context.WithValue(ctx, usageKey, u)
}

func UsageFrom(ctx context.Context) *Usage {
	u, _ := ctx.Value(usageKey).(*Usage)
	return u
}

// ValidationResult carries validation summary for context propagation across interceptors.
// The validation interceptor sets this; the trace interceptor reads it to populate span metadata.
type ValidationResult struct {
	Valid            bool
	ViolationCount   int
	EnforcementMode  string
	ValidatorName    string
	ValidatorVersion string
	RuleVersion      string
	DurationMs       int64
}

func WithValidationResult(ctx context.Context, v *ValidationResult) context.Context {
	return context.WithValue(ctx, validationMetaKey, v)
}

func ValidationResultFrom(ctx context.Context) *ValidationResult {
	v, _ := ctx.Value(validationMetaKey).(*ValidationResult)
	return v
}
