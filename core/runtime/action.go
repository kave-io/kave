// Package runtime holds the live, in-memory shapes used during action execution.
//
// Action vs Span — the two observability primitives in Kave:
//
//   - Action is intercept-mode. Kave sits in the request path (proxy/gateway);
//     before the upstream call it can authorize, enforce policies, deduct budget,
//     block. ActionRecord persists the full intercepted lifecycle (request,
//     response, decision, cost). Use Action when Kave CAN prevent the call.
//
//   - Span is observe-mode. An external runtime (LangChain, a customer SDK,
//     OTLP ingest) reports what already happened. Kave records it for cost,
//     tracing, and analytics but cannot block. Spans also exist inside the
//     intercept path as the observability slice of an Action — the pipeline
//     opens a span per Action to capture timing/cost/errors.
//
// Invariant: every Action produces at least one Span (the "action" span at the
// root of its trace). Nested spans (child tool calls, sub-LLM calls) share the
// Action's trace_id but are not themselves Actions unless Kave intercepted them.
package runtime

type ActionType string

const (
	TypeLLM       ActionType = "llm"
	TypeTool      ActionType = "tool"
	TypeRetrieval ActionType = "retrieval"
	TypeMutation  ActionType = "mutation"
	TypeAPI       ActionType = "api"
)

type ActionStatus string

const (
	StatusPending   ActionStatus = "pending"
	StatusRunning   ActionStatus = "running"
	StatusCompleted ActionStatus = "completed"
	StatusFailed    ActionStatus = "failed"
	StatusBlocked   ActionStatus = "blocked"
	StatusRetrying  ActionStatus = "retrying"
)

type ObservedActionStatus string

const (
	ObservedActionRunning   ObservedActionStatus = "running"
	ObservedActionCompleted ObservedActionStatus = "completed"
	ObservedActionFailed    ObservedActionStatus = "failed"
)

// ActionSource distinguishes intercepted (Kave in the causal path) from
// observed (agent-reported after the fact) actions.
type ActionSource string

const (
	ActionSourceIntercepted ActionSource = "intercepted"
	ActionSourceObserved    ActionSource = "observed"
)

// Outcome carries structured decision detail so auth/validate/cost can explain
// why an action was blocked, warned, denied, retried, or otherwise altered.
type Outcome struct {
	Code    string
	Message string
	Reason  string
}

// Action is an intercepted execution. Kave is in the causal path and can block it,
// but the agent/provider/tool still performs the actual work.
// Used in patterns 1 (HTTP proxy) and 3 (protocol bridge).
type Action struct {
	Invocation
	Status  ActionStatus
	Outcome *Outcome

	// TraceID is written by the pipeline when a new trace is created and read
	// by span persistence / trace export.
	TraceID string
	// SpanID is written by the pipeline for the current action span and read by
	// the tracer as the span identifier for this action's root node.
	SpanID string
	// ParentID is written by the pipeline from the active trace context and
	// read by the tracer to link child spans.
	ParentID string
}

// ObservedAction is an execution Kave learned about outside a blockable boundary.
// Kave cannot block it — auth violations are recorded, not enforced.
// Used in pattern 2 (SDK report-in).
type ObservedAction struct {
	Invocation
	Status ObservedActionStatus
}
