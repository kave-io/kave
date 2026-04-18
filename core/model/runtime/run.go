package runtime

import "github.com/kave-io/kave/core/pkg/money"

// Run status and trigger constants live in core/runtime (typed RunStatus
// and TriggerType). Model fields remain plain strings.

// RunRecord represents a persisted run record in the database.
// This is the storage/API shape.
// Compare with runtime.Run which is the live execution state used by handlers.
type RunRecord struct {
	ID               string
	ProjectID        string
	EnvID            string
	AgentID          string
	PolicyID         *string
	Name             string
	Status           string
	BudgetCap        money.Amount // 0 = no cap
	Spent            money.Amount
	DisplayBudgetCap *DisplayMoney // derived/UI-only; never canonical
	DisplaySpent     *DisplayMoney // derived/UI-only; never canonical
	Metadata         map[string]any

	ErrorMessage *string

	// Provenance — see runtime.TriggerType for valid values
	TriggerType    string
	TriggerID      *string // schedule ID, webhook ID, etc.
	CorrelationID  *string // external correlation (e.g. user session or request ID)
	SessionID      *string // groups related runs (e.g. a conversation thread)
	IdempotencyKey *string // client-supplied; unique per env; prevents double-runs

	StartedAt int64  // UnixMilli
	EndedAt   *int64 // UnixMilli; nil if still running
	CreatedAt int64  // UnixMilli
	UpdatedAt int64  // UnixMilli
}

// RunUpdate holds partial update fields for a run. Nil fields are not updated.
type RunUpdate struct {
	Status       *string
	Spent        *money.Amount
	DisplaySpent *DisplayMoney // derived/UI-only; never canonical
	ErrorMessage *string
	EndedAt      *int64
	Metadata     map[string]any
}

// RunFilter narrows ListRuns queries. Pagination (limit, cursor) is passed
// separately as a store.Page.
type RunFilter struct {
	ProjectID string
	EnvID     string
	AgentID   string
	Status    string
	FromMs    *int64
	ToMs      *int64
}
