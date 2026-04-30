package gateway

import (
	"encoding/json"
	"errors"
	"net/http"

	serverauth "github.com/kave-io/kave/server/ops/auth"
	serverbudget "github.com/kave-io/kave/server/ops/budget"
	serverpolicy "github.com/kave-io/kave/server/ops/policy"
)

const schemaVersion = 1

type errorPayload struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// ErrorEnvelope is the canonical JSON error response shape for the gateway.
// Exported so tests in package gateway can decode it.
type ErrorEnvelope struct {
	SchemaVersion int          `json:"schema_version"`
	Kind          string       `json:"kind"`
	Error         errorPayload `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	if code == "" {
		code = "unknown"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		SchemaVersion: schemaVersion,
		Kind:          "Error",
		Error:         errorPayload{Code: code, Message: message, Details: details},
	})
}

// Gateway-only sentinels. Policy and budget sentinels live with their
// pipeline stages (`server/ops/policy`, `server/ops/budget`) — the gateway
// no longer owns its own duplicates. mapError unwraps the typed errors
// from those packages directly.
var (
	ErrQuotaExceeded    = errors.New("gateway quota exceeded")
	ErrProviderNotFound = errors.New("gateway provider not supported")
	ErrUpstream         = errors.New("gateway upstream error")
)

func mapError(err error) (int, string, string, map[string]any) {
	switch {
	case errors.Is(err, serverpolicy.ErrPolicyBlocked):
		details := map[string]any{}
		message := serverpolicy.ErrPolicyBlocked.Error()
		var blocked *serverpolicy.BlockedError
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
		return 403, "gateway.policy_blocked", message, contractDetails(details)
	case errors.Is(err, serverbudget.ErrBudgetExceeded):
		details := map[string]any{}
		message := serverbudget.ErrBudgetExceeded.Error()
		var exceeded *serverbudget.ExceededError
		if errors.As(err, &exceeded) {
			message = exceeded.Error()
			if exceeded.Period != "" {
				details["period"] = exceeded.Period
			}
			if exceeded.Spent != 0 {
				details["spent"] = exceeded.Spent.String()
			}
			if exceeded.Limit != 0 {
				details["limit"] = exceeded.Limit.String()
			}
			if exceeded.Subject != "" {
				details["subject"] = exceeded.Subject
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
