package runtime

// Status and source constants for Action live in core/runtime (typed
// ActionStatus, ObservedActionStatus, ActionSource). Model fields remain
// plain strings; callers should cast the typed constants to string.

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

	// Status and source — see runtime.ActionStatus / runtime.ActionSource
	Status   string
	Source   string
	Metadata map[string]any

	// Retry/attempt metadata
	Attempt       int     // 1-based; 1 = first attempt
	MaxAttempts   int     // 0 = no retry configured
	RetryReason   *string // reason for this retry
	ProviderReqID *string // X-Request-ID from upstream provider (OpenAI, Anthropic, etc.)
	ExternalID    *string // client-supplied external correlation ID

	CreatedAt int64 // UnixMilli
}

// ActionUpdate holds terminal/action lifecycle fields updated after pipeline
// execution. Nil fields are preserved.
type ActionUpdate struct {
	Output        *[]byte
	Error         *string
	StartedAt     *int64
	EndedAt       *int64
	Status        *string
	Metadata      map[string]any
	ProviderReqID *string
}
