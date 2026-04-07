package intercept

import "github.com/kave-io/kave/core/pkg/timex"

// Unit is the shared data for any intercepted or observed execution.
// Embedded by Action and Event — not used directly.
type Unit struct {
	ID        string
	RunID     string
	ParentID  *string
	Type      ActionType
	Connector string
	Method    string
	Input     []byte // raw JSON
	Output    []byte // raw JSON
	Error     *string
	StartedAt timex.MS // zero = not set
	EndedAt   timex.MS // zero = not ended yet
	Depth     int      // 0 = root
	Seq       int      // sibling order within parent
}
