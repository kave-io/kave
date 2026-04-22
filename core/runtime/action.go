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

// Action is an execution Kave controls. It is in the causal path and can be blocked.
// Used in patterns 1 (HTTP proxy) and 3 (protocol bridge).
type Action struct {
	Invocation
	Status  ActionStatus
	Outcome *Outcome

	TraceID  string
	SpanID   string
	ParentID string
}

// ObservedAction is an execution the agent reported to Kave after the fact.
// Kave cannot block it — auth violations are recorded, not enforced.
// Used in pattern 2 (SDK report-in).
type ObservedAction struct {
	Invocation
	Status ObservedActionStatus
}
