package mappers

import (
	"testing"

	"github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/runtime/policy"
)

func TestBuildExecutionContextUsesRunFallback(t *testing.T) {
	run := &runtime.Run{
		ProjectID: "proj-1",
		EnvID:     "env-1",
		AgentID:   "ag-1",
	}
	p := &policy.Policy{ID: "pol-1"}

	ctx := BuildExecutionContext(run, p, "", "", "")
	if ctx == nil {
		t.Fatal("expected context")
	}
	if ctx.ProjectID != "proj-1" || ctx.EnvID != "env-1" || ctx.AgentID != "ag-1" {
		t.Fatalf("unexpected ids: project=%q env=%q agent=%q", ctx.ProjectID, ctx.EnvID, ctx.AgentID)
	}
}

func TestActionExecutionContextUsesRunAsAuthority(t *testing.T) {
	action := &runtime.Action{
		Invocation: runtime.Invocation{
			InvocationRef: runtime.InvocationRef{
				ProjectID: "proj-action",
				EnvID:     "env-action",
				AgentID:   "ag-action",
			},
		},
	}
	run := &runtime.Run{
		ProjectID: "proj-run",
		EnvID:     "env-run",
		AgentID:   "ag-run",
	}

	ctx := ActionExecutionContext(action, run, nil)
	if ctx == nil {
		t.Fatal("expected context")
	}
	if ctx.ProjectID != "proj-run" || ctx.AgentID != "ag-run" {
		t.Fatalf("expected run ids, got project=%q agent=%q", ctx.ProjectID, ctx.AgentID)
	}
}
