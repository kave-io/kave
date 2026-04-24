package control

import "github.com/kave-io/kave/core/pkg/money"

// PolicyStatus is the archival lifecycle of a PolicyRecord.
type PolicyStatus string

const (
	PolicyStatusActive   PolicyStatus = "active"
	PolicyStatusArchived PolicyStatus = "archived"
)

// Policy mode constants live in core/runtime/policy (typed policy.Mode).
// Model fields remain plain strings.

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
	// CasbinDocument stores an optional bespoke authorization document.
	// Empty means the allow-list fast path is used.
	CasbinDocument string

	// Budget and cost control
	BudgetCap      money.Amount // 0 = no cap
	BudgetPeriod   string       // "run" | "daily" | "monthly"; default "run"
	BudgetBehavior string       // "block" | "warn"; default "block"

	// Trace capture and retention
	TraceInput    bool // capture inputs; default true
	TraceOutput   bool // capture outputs; default true
	RetentionDays int  // span retention in days; default 30

	// Untyped extension point for validation rules and custom policies
	Config map[string]any

	Version   int    // incremented on every update; used for optimistic concurrency
	Mode      string // see policy.Mode for valid values
	Status    string // see PolicyStatus for valid values
	CreatedBy string // user ID or "system"
	UpdatedBy string // user ID or "system"
	CreatedAt int64  // UnixMilli
	UpdatedAt int64  // UnixMilli
}

// PolicyUpdate holds partial update fields for a policy. Nil/zero fields are not updated.
type PolicyUpdate struct {
	Description       *string
	AllowedTypes      []string
	AllowedConnectors []string
	AllowedMethods    []string
	CasbinDocument    *string
	BudgetCap         *money.Amount // nil = no change; use ClearBudgetCap to remove
	ClearBudgetCap    bool
	BudgetPeriod      *string
	BudgetBehavior    *string
	TraceInput        *bool
	TraceOutput       *bool
	RetentionDays     *int
	Config            map[string]any
	Mode              *string
	Status            *string
	UpdatedBy         *string
}
