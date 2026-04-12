package mappers

import (
	"github.com/kave-io/kave/core/ops/validate"
	"github.com/kave-io/kave/core/runtime"
)

// ValidationResultToOutcome maps validation failures to runtime outcomes.
// Returns nil when result is nil or valid.
func ValidationResultToOutcome(r *validate.Result) *runtime.Outcome {
	if r == nil || r.Valid {
		return nil
	}

	code := "validation_failed"
	message := "validation failed"
	reason := "validation failed"

	if len(r.Violations) > 0 {
		v := r.Violations[0]
		if v.Code != "" {
			code = v.Code
		}
		if v.Message != "" {
			message = v.Message
			reason = v.Message
		}
		if v.Path != "" {
			reason = v.Path
		}
	}

	return &runtime.Outcome{
		Code:    code,
		Message: message,
		Reason:  reason,
	}
}

// ValidationResultToMetadata maps validation details into metadata-safe shape.
func ValidationResultToMetadata(r *validate.Result) map[string]any {
	if r == nil {
		return nil
	}

	violations := make([]map[string]any, 0, len(r.Violations))
	for _, v := range r.Violations {
		violations = append(violations, map[string]any{
			"path":    v.Path,
			"code":    v.Code,
			"message": v.Message,
		})
	}

	return map[string]any{
		"valid":      r.Valid,
		"retryable":  r.Retryable,
		"violations": violations,
	}
}
