package intercept

import (
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/pkg/timex"
)

type RunStatus = string

const (
	RunActive    RunStatus = "active"
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
)

type Run struct {
	ID        string
	ProjectID string
	TapID     string
	PersonaID *string
	PolicyID  string
	Status    RunStatus
	StartedAt timex.MS // zero = not set
	EndedAt   timex.MS // zero = still running
	Spent     money.Amount
	Error     *string
	Metadata  map[string]any
}
