package mappers

import (
	"github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/runtime/policy"
)

// BuildExecutionContext builds runtime.ExecutionContext from run, policy, and optional IDs.
// Explicit IDs override run IDs when provided.
func BuildExecutionContext(run *runtime.Run, p *policy.Policy, projectID, envID, agentID string) *runtime.ExecutionContext {
	if run != nil {
		if projectID == "" {
			projectID = run.ProjectID
		}
		if envID == "" {
			envID = run.EnvID
		}
		if agentID == "" {
			agentID = run.AgentID
		}
	}

	return &runtime.ExecutionContext{
		ProjectID: projectID,
		EnvID:     envID,
		AgentID:   agentID,
		Policy:    p,
		Run:       run,
	}
}

// ActionExecutionContext builds runtime.ExecutionContext from action/run/policy.
// Run values win over action values because run is the authoritative execution root.
func ActionExecutionContext(action *runtime.Action, run *runtime.Run, p *policy.Policy) *runtime.ExecutionContext {
	if run != nil {
		return BuildExecutionContext(run, p, "", "", "")
	}

	projectID := ""
	envID := ""
	agentID := ""
	if action != nil {
		projectID = action.ProjectID
		envID = action.EnvID
		agentID = action.AgentID
	}
	return BuildExecutionContext(run, p, projectID, envID, agentID)
}
