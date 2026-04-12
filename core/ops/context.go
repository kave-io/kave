package ops

import (
	"github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/runtime/policy"
)

// ExecutionContext bundles project, env, policy, agent, and run for operation interfaces.
// Passed to all interceptor Before/After hooks for consistent context.
type ExecutionContext struct {
	ProjectID string
	EnvID     string
	AgentID   string
	Policy    *policy.Policy
	Run       *runtime.Run
}
