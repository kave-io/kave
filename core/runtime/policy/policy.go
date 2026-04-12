package policy

import (
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/pkg/timex"
)

// Policy is the runtime policy composition root.
type Policy struct {
	ID        string
	ProjectID string
	Name      string
	Mode      string // "enforce" | "shadow"; default enforce

	Auth       *AuthPolicy
	Cost       *CostPolicy
	Trace      *TracePolicy
	Validation *ValidationPolicy

	CreatedAt timex.MS
	UpdatedAt timex.MS
}

// AuthPolicy owns auth-specific allow/deny rules.
type AuthPolicy struct {
	PolicyID          string
	AllowedTypes      []string
	AllowedConnectors []string
	AllowedMethods    []string
}

// CostPolicy owns budget and spending rules.
type CostPolicy struct {
	PolicyID       string
	BudgetCap      *money.Amount
	BudgetPeriod   string
	BudgetBehavior string
}

// TracePolicy owns trace capture and retention rules.
type TracePolicy struct {
	PolicyID      string
	Input         bool
	Output        bool
	RetentionDays int
}

// ValidationPolicy owns validation-specific settings.
type ValidationPolicy struct {
	PolicyID  string
	Enabled   bool
	Retryable bool
	Config    map[string]any
}
