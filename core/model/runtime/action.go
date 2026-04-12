package runtime

// Action source constants — distinguishes intercepted from observed actions.
const (
	ActionSourceIntercepted = "intercepted" // Kave is in the causal path; can block
	ActionSourceObserved    = "observed"    // agent-reported after the fact; audit only
)

// Action status constants.
const (
	ActionStatusPending   = "pending"
	ActionStatusRunning   = "running"
	ActionStatusCompleted = "completed"
	ActionStatusFailed    = "failed"
	ActionStatusBlocked   = "blocked"
	ActionStatusRetrying  = "retrying"
)

// ActionRecord is the persisted representation of an intercepted or observed action.
// Maps to a single table; Source discriminates intercepted from observed.
type ActionRecord struct {
	// Ref: identity and topology
	ID        string
	RunID     string
	AgentID   string
	ProjectID string
	EnvID     string
	ParentID  *string

	// Target: what operation
	ActionType string
	Connector  string
	Method     string

	// Data: payloads and error
	// Input and Output are nullable: nil = not captured, []byte{} = captured as empty
	Input  *[]byte
	Output *[]byte
	Error  *string

	// Timing and ordering
	StartedAt *int64 // UnixMilli; nil = action not started
	EndedAt   *int64 // UnixMilli; nil = still running
	Depth     int    // 0 = root
	Seq       int    // sibling order

	// Status and source
	Status   string // ActionStatus* constants
	Source   string // ActionSource* constants
	Metadata map[string]any

	// Retry/attempt metadata
	Attempt       int     // 1-based; 1 = first attempt
	MaxAttempts   int     // 0 = no retry configured
	RetryReason   *string // reason for this retry
	ProviderReqID *string // X-Request-ID from upstream provider (OpenAI, Anthropic, etc.)
	ExternalID    *string // client-supplied external correlation ID

	CreatedAt int64 // UnixMilli
}
