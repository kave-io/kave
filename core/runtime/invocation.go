package runtime

import "github.com/kave-io/kave/core/pkg/timex"

// InvocationRef carries identity and topology for one unit of work.
type InvocationRef struct {
	ID        string
	RunID     string
	AgentID   string
	ProjectID string
	EnvID     string
	ParentID  *string
}

// InvocationTarget describes the operation being performed.
type InvocationTarget struct {
	Type      ActionType
	Connector string
	Method    string
}

// InvocationData carries payloads and terminal error state.
// Input and Output are nullable: nil = not captured, []byte{} = captured as empty
type InvocationData struct {
	Input  *[]byte // raw JSON; nil if not captured by policy
	Output *[]byte // raw JSON; nil if not captured or still pending
	Error  *string
}

// InvocationTiming carries start/end timing and ordering metadata.
type InvocationTiming struct {
	StartedAt timex.MS
	EndedAt   *timex.MS
	Depth     int // 0 = root
	Seq       int // sibling order
}

// Invocation is the shared data for any intercepted or observed execution.
// Not used directly — embedded by Action and ObservedAction.
type Invocation struct {
	InvocationRef
	InvocationTarget
	InvocationData
	InvocationTiming
}
