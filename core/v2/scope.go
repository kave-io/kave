// Package v2 contains the small, transport-neutral domain for Kave's V2
// admission and usage kernel. It deliberately has no knowledge of users,
// subscriptions, prompts, provider SDKs, or storage backends.
package v2

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const maxRefLength = 160
const maxNameLength = 128

var (
	refPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]*$`)
	namePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	metricPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)

	ErrInvalidArgument     = errors.New("kave v2: invalid argument")
	ErrLimitExceeded       = errors.New("kave v2: limit exceeded")
	ErrIdempotencyConflict = errors.New("kave v2: idempotency conflict")
)

// Ref is a bounded, header-safe, pseudonymous reference supplied by an
// application. Kave treats it as opaque and never resolves it to a person.
type Ref string

func (r Ref) Validate(field string, required bool) error {
	value := string(r)
	if value == "" {
		if required {
			return invalid(field, "is required")
		}
		return nil
	}
	if len(value) > maxRefLength {
		return invalid(field, fmt.Sprintf("must be at most %d bytes", maxRefLength))
	}
	if strings.TrimSpace(value) != value || !refPattern.MatchString(value) {
		return invalid(field, "must be an opaque reference using letters, digits, '.', '_', ':', '/', '@', or '-'")
	}
	return nil
}

// ValidateName validates a path-safe, database-safe static resource name.
// Unlike opaque tenant refs, names cannot contain '/', ':', or '@'.
func (r Ref) ValidateName(field string, required bool) error {
	value := string(r)
	if value == "" {
		if required {
			return invalid(field, "is required")
		}
		return nil
	}
	if len(value) > maxNameLength {
		return invalid(field, fmt.Sprintf("must be at most %d bytes", maxNameLength))
	}
	if !namePattern.MatchString(value) {
		return invalid(field, "must be path-safe and contain only letters, digits, '.', '_', or '-'")
	}
	return nil
}

// Scope is asserted by a trusted service key for one operation. Tenant and
// billing-subject are deliberately separate: a Solo subscription can be billed
// to a user while a clinic request is billed to a clinic.
type Scope struct {
	Tenant  Ref `json:"tenant,omitempty"`
	Actor   Ref `json:"actor,omitempty"`
	BillTo  Ref `json:"bill_to,omitempty"`
	Session Ref `json:"session,omitempty"`
	Feature Ref `json:"feature,omitempty"`
}

// Validate rejects values that could smuggle additional HTTP headers or create
// unbounded cardinality. It is intentionally structural; admission operations
// use ValidateAdmission to require the tenant and billing boundaries that make
// hierarchical limits non-bypassable.
func (s Scope) Validate() error {
	for _, item := range []struct {
		name  string
		value Ref
	}{
		{name: "scope.tenant", value: s.Tenant},
		{name: "scope.actor", value: s.Actor},
		{name: "scope.bill_to", value: s.BillTo},
		{name: "scope.session", value: s.Session},
		{name: "scope.feature", value: s.Feature},
	} {
		if err := item.value.Validate(item.name, false); err != nil {
			return err
		}
	}
	return nil
}

// ValidateAdmission validates a scope asserted for Consume or provider
// invocation. Tenant and billing subject are mandatory even for system work;
// callers should use explicit opaque values such as "system/indexer" instead
// of omitting a dimension and accidentally escaping scoped limits.
func (s Scope) ValidateAdmission() error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := s.Tenant.Validate("scope.tenant", true); err != nil {
		return err
	}
	return s.BillTo.Validate("scope.bill_to", true)
}

// Metric is a measured quota or usage dimension. Applications may define
// product metrics such as "ai_actions" alongside Kave's provider metrics.
type Metric string

const (
	MetricRequests     Metric = "requests"
	MetricInputTokens  Metric = "input_tokens"
	MetricOutputTokens Metric = "output_tokens"
	MetricCostNanoUSD  Metric = "cost_nano_usd"
)

func (m Metric) Validate() error {
	value := string(m)
	if len(value) == 0 || len(value) > 64 || !metricPattern.MatchString(value) {
		return invalid("metric", "must start with a lowercase letter and contain only lowercase letters, digits, '.', '_', or '-'")
	}
	return nil
}

func invalid(field, reason string) error {
	return fmt.Errorf("%w: %s %s", ErrInvalidArgument, field, reason)
}
