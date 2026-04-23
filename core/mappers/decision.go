package mappers

import (
	"github.com/kave-io/kave/core/runtime/auth"
	"github.com/kave-io/kave/core/runtime"
)

// AuthDecisionToOutcome maps auth decision results to runtime outcome fields.
func AuthDecisionToOutcome(d *auth.Decision) *runtime.Outcome {
	if d == nil {
		return nil
	}

	code := d.Code
	if code == "" {
		if d.Allowed {
			code = "allow"
		} else {
			code = "deny"
		}
	}

	message := d.Reason
	if message == "" {
		if d.Allowed {
			message = "allowed"
		} else {
			message = "denied"
		}
	}

	return &runtime.Outcome{
		Code:    code,
		Message: message,
		Reason:  d.Reason,
	}
}
