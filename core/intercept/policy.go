package intercept

import (
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/pkg/timex"
)

type Policy struct {
	ID        string
	ProjectID string
	Name      string

	AllowedTypes      []string // ["*"] or ["llm","tool"]
	AllowedConnectors []string // ["*"] or ["openai","stripe"]
	AllowedMethods    []string // ["*"] or ["chat.completions"]

	BudgetCap      *money.Amount // nil = unlimited
	BudgetPeriod   string        // "run" | "daily" | "monthly"
	BudgetBehavior string        // "block" | "warn"

	TraceInput    bool
	TraceOutput   bool
	RetentionDays int

	Validation map[string]any // v2 placeholder

	CreatedAt timex.MS
	UpdatedAt timex.MS
}
