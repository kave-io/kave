package control

import "github.com/kave-io/kave/core/pkg/money"

// Agent status constants.
const (
	AgentStatusActive   = "active"
	AgentStatusDisabled = "disabled"
)

// Agent is a registered AI agent identity.
type Agent struct {
	ID            string
	ProjectID     string
	EnvID         string
	Name          string
	Description   string
	PolicyID      *string
	MonthlyBudget *money.Amount
	Status        string // AgentStatusActive | AgentStatusDisabled
	Metadata      map[string]any
	CreatedBy     string // user ID or "system"
	UpdatedBy     string // user ID or "system"
	DeletedAt     *int64 // UnixMilli; nil = not deleted
	CreatedAt     int64  // UnixMilli
	UpdatedAt     int64  // UnixMilli
}

// AgentUpdate holds partial update fields for an agent. Nil fields are not updated.
type AgentUpdate struct {
	Description   *string
	PolicyID      *string
	MonthlyBudget *money.Amount
	Status        *string
	Metadata      map[string]any
	UpdatedBy     *string
}
