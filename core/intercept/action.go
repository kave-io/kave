package intercept

type ActionType = string

const (
	TypeLLM       ActionType = "llm"
	TypeTool      ActionType = "tool"
	TypeRetrieval ActionType = "retrieval"
	TypeMutation  ActionType = "mutation"
	TypeAPI       ActionType = "api"
)

type ActionStatus = string

const (
	StatusPending   ActionStatus = "pending"
	StatusRunning   ActionStatus = "running"
	StatusCompleted ActionStatus = "completed"
	StatusFailed    ActionStatus = "failed"
	StatusBlocked   ActionStatus = "blocked"
)

type EventStatus = string

const (
	EventRunning   EventStatus = "running"
	EventCompleted EventStatus = "completed"
	EventFailed    EventStatus = "failed"
)

// Action is an execution Kave controls. It is in the causal path and can be blocked.
// Used in patterns 1 (HTTP proxy) and 3 (protocol bridge).
type Action struct {
	Unit
	Status ActionStatus
}

// Event is an execution the agent reported to Kave after the fact.
// Kave cannot block it — auth violations are recorded, not enforced.
// Used in pattern 2 (SDK report-in).
type Event struct {
	Unit
	Status EventStatus
}
