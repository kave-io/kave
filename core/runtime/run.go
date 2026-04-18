package runtime

import (
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/pkg/timex"
)

type RunStatus string

const (
	RunActive    RunStatus = "active"
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
	RunTimedOut  RunStatus = "timed_out"
	RunBlocked   RunStatus = "blocked"
)

// TriggerType is the origin of a Run.
type TriggerType string

const (
	TriggerAPI      TriggerType = "api"
	TriggerSchedule TriggerType = "schedule"
	TriggerWebhook  TriggerType = "webhook"
	TriggerManual   TriggerType = "manual"
)

type Run struct {
	ID        string
	ProjectID string
	EnvID     string
	AgentID   string
	PolicyID  *string
	Name      string
	Status    RunStatus
	StartedAt timex.MS // zero = not set
	EndedAt   timex.MS // zero = still running
	Spent     money.Amount
	Error     *string
	Metadata  map[string]any

	// Provenance — set once at run creation, never updated
	TriggerType   TriggerType
	CorrelationID *string
	SessionID     *string
}
