package watch

import "testing"

func TestMatchFilter(t *testing.T) {
	ev := Event{AgentID: "agent-1", RunID: "run-1", TraceID: "trace-1", Status: "completed", Kind: "run.completed"}
	f := Filter{Agent: "agent-1", RunID: "run-1", TraceID: "trace-1", Status: "completed", Type: "run"}
	if !matchFilter(ev, f) {
		t.Fatalf("expected match")
	}
	if matchFilter(ev, Filter{Agent: "agent-2"}) {
		t.Fatalf("unexpected match")
	}
}
