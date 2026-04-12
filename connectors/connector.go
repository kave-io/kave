package connectors

import "github.com/kave-io/kave/core/ports"

// Re-export connector contracts from core/ports so connectors can implement
// the interface without depending on server packages.
type (
	Capabilities = ports.Capabilities
	Handler      = ports.Handler
	Connector    = ports.Connector
	Kind         = ports.Kind
)
