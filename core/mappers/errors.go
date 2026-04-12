package mappers

import "fmt"

// MappingError indicates a conversion failed between domains.
type MappingError struct {
	Domain string // "runtime_to_model", "model_to_runtime", "proto_to_runtime"
	Type   string // "Action", "Run", "Policy"
	Reason string
}

func (e *MappingError) Error() string {
	return fmt.Sprintf("mapper: %s %s → %s failed: %s", e.Domain, e.Type, e.Type, e.Reason)
}

func newError(domain, typ, reason string) *MappingError {
	return &MappingError{Domain: domain, Type: typ, Reason: reason}
}
