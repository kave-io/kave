package validate

import (
	"context"

	"github.com/kave-io/kave/core/runtime"
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

// Result is the structured outcome of validation.
// Meta mirrors Valid/Retryable/Violation count for persistence; nil = validation did not run.
type Result struct {
	Valid      bool
	Violations []Violation
	Retryable  bool
	Meta       *runtime.ValidationMeta
}

// Validator checks action output against a schema.
type Validator interface {
	Validate(ctx context.Context, output []byte, schema *Schema, policy *policy.ValidationPolicy) (*Result, error)
}
