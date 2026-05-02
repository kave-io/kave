package watch

import "testing"

func TestApplyStats(t *testing.T) {
	stats := Stats{}
	stats = ApplyStats(stats, Event{Kind: "run.started", Status: "active"})
	stats = ApplyStats(stats, Event{Kind: "run.completed", Status: "completed", Cost: 0.25, Currency: "USD"})
	stats = ApplyStats(stats, Event{Kind: "policy.blocked", PolicyDecision: "deny"})
	stats = ApplyStats(stats, Event{Kind: "action.failed", Error: "boom"})

	if stats.ActiveRuns != 1 {
		t.Fatalf("active runs: got %d want 1", stats.ActiveRuns)
	}
	if stats.CompletedRuns != 1 {
		t.Fatalf("completed runs: got %d want 1", stats.CompletedRuns)
	}
	if stats.BlockedOrDenied != 1 {
		t.Fatalf("blocked runs: got %d want 1", stats.BlockedOrDenied)
	}
	if stats.Errors != 1 {
		t.Fatalf("errors: got %d want 1", stats.Errors)
	}
	if stats.TotalCost != 0.25 {
		t.Fatalf("total cost: got %v want 0.25", stats.TotalCost)
	}
}
