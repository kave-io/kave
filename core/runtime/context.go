package runtime

import (
	"context"

	"github.com/kave-io/kave/core/runtime/policy"
)

type ctxKey int

const (
	policyKey ctxKey = iota
	runKey
	usageKey
	validationMetaKey
	traceKey
)

// TokenUsage carries token counts from an LLM response.
// The proxy handler sets this on the context so cost/trace interceptors can read it.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	CacheRead    int
	CacheWrite   int
	Reasoning    int // output reasoning tokens (OpenAI o1/o3, Claude thinking)
	AudioInput   int // audio input tokens (OpenAI audio models)
	AudioOutput  int // audio output tokens (OpenAI audio models)
	ImageUnits   int // provider-agnostic media unit count (OpenAI image_tokens, etc.)
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

// TraceContext captures the current trace and span identity.
type TraceContext struct {
	TraceID    string
	SpanID     string
	RootSpanID string
}

// WithTrace attaches the active trace/span to the context.
func WithTrace(ctx context.Context, traceID, spanID string) context.Context {
	tc := TraceContext{TraceID: traceID, SpanID: spanID, RootSpanID: spanID}
	if prev, ok := ctx.Value(traceKey).(TraceContext); ok {
		tc.RootSpanID = prev.RootSpanID
		if tc.RootSpanID == "" {
			tc.RootSpanID = prev.SpanID
		}
	}
	return context.WithValue(ctx, traceKey, tc)
}

// TraceFrom returns the active trace context, if present.
func TraceFrom(ctx context.Context) TraceContext {
	tc, _ := ctx.Value(traceKey).(TraceContext)
	return tc
}

// WithUsage attaches per-action billable usage to the context. Usage.Tokens carries
// LLM token counts; use that rather than a separate TokenUsage context key.
func WithUsage(ctx context.Context, u *Usage) context.Context {
	return context.WithValue(ctx, usageKey, u)
}

// UsageFrom returns the Usage previously attached via WithUsage, or nil.
func UsageFrom(ctx context.Context) *Usage {
	u, _ := ctx.Value(usageKey).(*Usage)
	return u
}

// TokenUsageFrom is a convenience for reading just the token portion of Usage.
// Returns nil if no Usage is attached or if it has no token counts.
func TokenUsageFrom(ctx context.Context) *TokenUsage {
	if u := UsageFrom(ctx); u != nil {
		return u.Tokens
	}
	return nil
}

// ValidationMeta is the canonical validation summary. It carries execution
// provenance and is propagated across interceptors (validation → trace) and
// persisted as part of the span record.
type ValidationMeta struct {
	Valid            bool
	ViolationCount   int
	EnforcementMode  string // "block" | "warn" | "audit"
	ValidatorName    string
	ValidatorVersion string
	RuleVersion      string
	DurationMs       int64
	Retryable        bool
}

func WithValidationMeta(ctx context.Context, v *ValidationMeta) context.Context {
	return context.WithValue(ctx, validationMetaKey, v)
}

func ValidationMetaFrom(ctx context.Context) *ValidationMeta {
	v, _ := ctx.Value(validationMetaKey).(*ValidationMeta)
	return v
}

// ExecutionContext bundles project, env, policy, agent, and run for operation
// interfaces. Passed to interceptor Before/After hooks for consistent context.
type ExecutionContext struct {
	ProjectID string
	EnvID     string
	AgentID   string
	Policy    *policy.Policy
	Run       *Run
}
