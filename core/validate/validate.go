package validate

import "context"

type Schema struct {
	Name       string
	Definition map[string]any
}

// Validator checks action output against a schema.
// Implementations (JSON Schema, regex, guardrails) live in server/.
type Validator interface {
	Validate(ctx context.Context, output []byte, schema *Schema) error
}
