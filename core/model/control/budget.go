package control

import "github.com/kave-io/kave/core/pkg/money"

// BudgetPeriod controls the reset cadence for an agent budget.
const (
	BudgetPeriodRun     = "run"
	BudgetPeriodDaily   = "daily"
	BudgetPeriodMonthly = "monthly"
)

// Budget is a per-agent spend cap configuration.
type Budget struct {
	ID        string
	AgentID   string
	HardCap   money.Amount
	SoftCap   money.Amount
	Period    string
	CreatedAt int64 // UnixMilli
	UpdatedAt int64 // UnixMilli
}
