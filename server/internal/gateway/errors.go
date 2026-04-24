package gateway

import (
	"errors"
	"fmt"

	"github.com/kave-io/kave/core/pkg/money"
	serverauth "github.com/kave-io/kave/server/ops/auth"
	serverbudget "github.com/kave-io/kave/server/ops/budget"
	serverpolicy "github.com/kave-io/kave/server/ops/policy"
)

var (
	ErrPolicyBlocked    = errors.New("gateway policy blocked")
	ErrBudgetExceeded   = errors.New("gateway budget exceeded")
	ErrQuotaExceeded    = errors.New("gateway quota exceeded")
	ErrProviderNotFound = errors.New("gateway provider not supported")
	ErrUpstream         = errors.New("gateway upstream error")
)

// PolicyBlockedError carries the policy decision detail.
type PolicyBlockedError struct {
	Reason  string
	Subject string
	Object  string
}

func (e *PolicyBlockedError) Error() string {
	if e == nil {
		return ErrPolicyBlocked.Error()
	}
	if e.Reason != "" {
		return e.Reason
	}
	return ErrPolicyBlocked.Error()
}

func (e *PolicyBlockedError) Unwrap() error { return ErrPolicyBlocked }

// BudgetExceededError carries budget spend and limit detail.
type BudgetExceededError struct {
	Spent   money.Amount
	Limit   money.Amount
	Period  string
	Subject string
}

func (e *BudgetExceededError) Error() string {
	if e == nil {
		return ErrBudgetExceeded.Error()
	}
	return fmt.Sprintf("budget exceeded: spent=%s limit=%s period=%s", e.Spent.String(), e.Limit.String(), e.Period)
}

func (e *BudgetExceededError) Unwrap() error { return ErrBudgetExceeded }

func mapError(err error) (int, string, string, map[string]any) {
	switch {
	case errors.Is(err, ErrPolicyBlocked), errors.Is(err, serverpolicy.ErrPolicyBlocked):
		var blocked *PolicyBlockedError
		details := map[string]any{}
		message := ErrPolicyBlocked.Error()
		if errors.As(err, &blocked) {
			if blocked.Reason != "" {
				message = blocked.Reason
			}
			if blocked.Subject != "" {
				details["subject"] = blocked.Subject
			}
			if blocked.Object != "" {
				details["object"] = blocked.Object
			}
		}
		var policyBlocked *serverpolicy.BlockedError
		if errors.As(err, &policyBlocked) {
			if policyBlocked.Reason != "" {
				message = policyBlocked.Reason
			}
			if policyBlocked.Subject != "" {
				details["subject"] = policyBlocked.Subject
			}
			if policyBlocked.Object != "" {
				details["object"] = policyBlocked.Object
			}
		}
		return 403, "gateway.policy_blocked", message, contractDetails(details)
	case errors.Is(err, ErrBudgetExceeded), errors.Is(err, serverbudget.ErrBudgetExceeded):
		var blocked *BudgetExceededError
		details := map[string]any{}
		message := ErrBudgetExceeded.Error()
		if errors.As(err, &blocked) {
			message = blocked.Error()
			if blocked.Period != "" {
				details["period"] = blocked.Period
			}
			if blocked.Spent != 0 {
				details["spent"] = blocked.Spent.String()
			}
			if blocked.Limit != 0 {
				details["limit"] = blocked.Limit.String()
			}
			if blocked.Subject != "" {
				details["subject"] = blocked.Subject
			}
		}
		var budgetExceeded *serverbudget.ExceededError
		if errors.As(err, &budgetExceeded) {
			message = budgetExceeded.Error()
			if budgetExceeded.Period != "" {
				details["period"] = budgetExceeded.Period
			}
			if budgetExceeded.Spent != 0 {
				details["spent"] = budgetExceeded.Spent.String()
			}
			if budgetExceeded.Limit != 0 {
				details["limit"] = budgetExceeded.Limit.String()
			}
			if budgetExceeded.Subject != "" {
				details["subject"] = budgetExceeded.Subject
			}
		}
		return 402, "gateway.budget_exceeded", message, contractDetails(details)
	case errors.Is(err, ErrQuotaExceeded):
		return 429, "gateway.quota_exceeded", err.Error(), nil
	case errors.Is(err, serverauth.ErrUnauthenticated):
		return 401, "gateway.unauthorized", err.Error(), nil
	case errors.Is(err, serverauth.ErrUnauthorized):
		return 403, "gateway.policy_blocked", err.Error(), nil
	case errors.Is(err, ErrProviderNotFound):
		return 400, "gateway.provider_not_supported", err.Error(), nil
	case errors.Is(err, ErrUpstream):
		return 502, "gateway.upstream_error", err.Error(), nil
	default:
		return 500, "gateway.internal_error", err.Error(), nil
	}
}

func contractDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return nil
	}
	return details
}
