package cost

import "github.com/kave-io/kave/core/pkg/money"

// BudgetStatus is the result of evaluating spend against a policy cap.
type BudgetStatus struct {
	Spent     money.Amount
	Cap       *money.Amount // nil = unlimited
	Period    string
	Remaining *money.Amount // nil if no cap
	Exceeded  bool
}

// NewBudgetStatus computes a correct BudgetStatus from spent amount, optional cap, and period.
// Pure function — no DB, no side effects.
func NewBudgetStatus(spent money.Amount, cap *money.Amount, period string) *BudgetStatus {
	bs := &BudgetStatus{
		Spent:  spent,
		Cap:    cap,
		Period: period,
	}
	if cap != nil {
		remaining := *cap - spent
		bs.Remaining = &remaining
		bs.Exceeded = spent >= *cap
	}
	return bs
}

// Meter prices actions and evaluates budget.
// Takes primitives — the server-side interceptor extracts values from Run and Policy.
type Meter interface {
	// Cost returns the cost for one action given token counts.
	Cost(connector, model string, inputTokens, outputTokens int) money.Amount

	// Budget evaluates the current budget status. Does not write anything.
	Budget(spent money.Amount, cap *money.Amount, period string) *BudgetStatus
}
