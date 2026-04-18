package policy

import (
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/pkg/timex"
)

// Mode is how policy violations are handled at runtime.
type Mode string

const (
	ModeEnforce Mode = "enforce" // violations block the action
	ModeShadow  Mode = "shadow"  // violations are recorded but not blocked
)

// BudgetPeriod is the rollover window for a budget cap.
type BudgetPeriod string

const (
	BudgetPerRun     BudgetPeriod = "run"
	BudgetPerDaily   BudgetPeriod = "daily"
	BudgetPerMonthly BudgetPeriod = "monthly"
)

// BudgetBehavior is what happens when a budget cap is exceeded.
type BudgetBehavior string

const (
	BudgetBlock BudgetBehavior = "block"
	BudgetWarn  BudgetBehavior = "warn"
)

// Policy is the runtime policy composition root.
// Mode defaults to ModeEnforce when empty.
type Policy struct {
	ID        string
	ProjectID string
	Name      string
	Mode      Mode

	Auth       *AuthPolicy
	Cost       *CostPolicy
	Trace      *TracePolicy
	Validation *ValidationPolicy

	CreatedAt timex.MS
	UpdatedAt timex.MS
}

// AuthPolicy owns auth-specific allow/deny rules.
// The owning Policy.ID provides identity; sub-policies carry only their fields.
type AuthPolicy struct {
	AllowedTypes      []string
	AllowedConnectors []string
	AllowedMethods    []string
}

// CostPolicy owns budget and spending rules.
type CostPolicy struct {
	BudgetCap      *money.Amount
	BudgetPeriod   BudgetPeriod
	BudgetBehavior BudgetBehavior
}

// TracePolicy owns trace capture and retention rules.
type TracePolicy struct {
	Input         bool
	Output        bool
	RetentionDays int
}

// ValidationPolicy owns validation-specific settings.
type ValidationPolicy struct {
	Enabled   bool
	Retryable bool
	Config    map[string]any
}
