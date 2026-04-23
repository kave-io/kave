package cost

import (
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/runtime/policy"
)

// BudgetStatus is the result of evaluating spend against a policy cap.
type BudgetStatus struct {
	Spent     money.Amount
	Cap       *money.Amount // nil = unlimited
	Period    string
	Remaining *money.Amount // nil if no cap
	Exceeded  bool
}

// NewBudgetStatus computes a BudgetStatus from spent amount, optional cap, and period.
func NewBudgetStatus(spent money.Amount, cap *money.Amount, period string) *BudgetStatus {
	bs := &BudgetStatus{
		Spent:  spent,
		Cap:    cap,
		Period: period,
	}
	if cap != nil {
		remaining, _ := (*cap).Sub(spent)
		bs.Remaining = &remaining
		bs.Exceeded = spent >= *cap
	}
	return bs
}

// CostUsage carries all billable dimensions for a single action.
// Token fields correspond to provider-reported usage; nil = not applicable for this action type.
type CostUsage struct {
	Input          int
	Output         int
	CacheRead      int
	CacheWrite     int
	Reasoning      int   // o1/o3, Claude thinking
	AudioInput     int   // OpenAI audio models
	AudioOutput    int   // OpenAI audio models
	ImageUnits     int   // provider-agnostic image tiles/units
	RequestCount   int   // per-request billing (some tools)
	ComputeMs      int64 // server-side compute time (Replicate, Modal)
	StorageBytes   int64 // bytes stored (vector DBs)
	BandwidthBytes int64 // bytes transferred (outbound calls)
}

// Pricer calculates the cost of an action from its usage dimensions.
type Pricer interface {
	Cost(connector, model string, usage *CostUsage) money.Amount
}

// BudgetEvaluator evaluates a spend amount against a policy cap.
type BudgetEvaluator interface {
	Budget(spent money.Amount, policy *policy.CostPolicy) *BudgetStatus
}

// Meter composes pricing and budget evaluation for convenience.
type Meter interface {
	Pricer
	BudgetEvaluator
}
