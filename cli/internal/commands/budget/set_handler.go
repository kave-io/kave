package budget

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	"github.com/kave-io/kave/proto/gen/kave/common/v1"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type SetInput struct {
	AgentID string
	HardCap string // decimal amount as string
	SoftCap string // decimal amount as string
	Period  string // billing period
}

type SetOutput struct {
	Data *controlv1.Budget `json:"data"`
}

func RunSet(ctx context.Context, in SetInput) (*SetOutput, error) {
	if in.AgentID == "" || in.HardCap == "" {
		return nil, fmt.Errorf("--agent and --hard-cap are required")
	}
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	t, err := rt.GetTransport()
	if err != nil {
		return nil, err
	}
	svc, err := t.ControlSvc()
	if err != nil {
		return nil, err
	}

	hardCap := &commonv1.Amount{
		Decimal: in.HardCap,
	}

	var softCap *commonv1.Amount
	if in.SoftCap != "" {
		softCap = &commonv1.Amount{
			Decimal: in.SoftCap,
		}
	}

	// Map period string to enum
	period := controlv1.BudgetPeriod_BUDGET_PERIOD_UNSPECIFIED
	if in.Period != "" {
		switch in.Period {
		case "run":
			period = controlv1.BudgetPeriod_BUDGET_PERIOD_RUN
		case "daily":
			period = controlv1.BudgetPeriod_BUDGET_PERIOD_DAILY
		case "monthly":
			period = controlv1.BudgetPeriod_BUDGET_PERIOD_MONTHLY
		default:
			period = controlv1.BudgetPeriod_BUDGET_PERIOD_UNSPECIFIED
		}
	}

	rec, err := svc.CreateBudget(ctx, &controlv1.CreateBudgetRequest{
		AgentId: in.AgentID,
		HardCap: hardCap,
		SoftCap: softCap,
		Period:  period,
	})
	if err != nil {
		return nil, err
	}
	return &SetOutput{Data: rec}, nil
}
