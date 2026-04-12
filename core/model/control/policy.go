package control

import "github.com/kave-io/kave/core/pkg/money"

// Policy mode constants.
const (
	PolicyModeEnforce = "enforce" // violations block the action
	PolicyModeShadow  = "shadow"  // violations are recorded but not blocked
)

// Policy status constants.
const (
	PolicyStatusActive   = "active"
	PolicyStatusArchived = "archived"
)

// PolicyRecord defines what an agent is allowed to do.
type PolicyRecord struct {
	ID                string
	ProjectID         string
	EnvID             string
	Name              string
	Description       string
	AllowedTypes      []string
	AllowedConnectors []string
	AllowedMethods    []string

	// Budget and cost control
	BudgetCap      money.Amount // 0 = no cap
	BudgetPeriod   string        // "run" | "daily" | "monthly"; default "run"
	BudgetBehavior string        // "block" | "warn"; default "block"

	// Trace capture and retention
	TraceInput    bool // capture inputs; default true
	TraceOutput   bool // capture outputs; default true
	RetentionDays int  // span retention in days; default 30

	// Untyped extension point for validation rules and custom policies
	Config map[string]any

	Version   int    // incremented on every update; used for optimistic concurrency
	Mode      string // PolicyModeEnforce | PolicyModeShadow
	Status    string // PolicyStatusActive | PolicyStatusArchived
	CreatedBy string // user ID or "system"
	UpdatedBy string // user ID or "system"
	CreatedAt int64  // UnixMilli
	UpdatedAt int64  // UnixMilli
}
