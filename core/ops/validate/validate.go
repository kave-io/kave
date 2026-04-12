package validate

import (
	"context"

	"github.com/kave-io/kave/core/runtime/policy"
)

type Schema struct {
	Name       string
	Definition map[string]any
}

// Violation describes a validation issue at a specific path.
type Violation struct {
	Path    string
	Code    string
	Message string
}

// ExecutionMeta carries validation execution provenance.
type ExecutionMeta struct {
	ValidatorName    string
	ValidatorVersion string
	RuleVersion      string
	EnforcementMode  string // "block" | "warn" | "audit"
	DurationMs       int64
}

// Result is the structured outcome of validation.
type Result struct {
	Valid      bool
	Violations []Violation
	Retryable  bool
	Meta       *ExecutionMeta // nil = validation did not run
}

// Validator checks action output against a schema.
type Validator interface {
	Validate(ctx context.Context, output []byte, schema *Schema, policy *policy.ValidationPolicy) (*Result, error)
}
