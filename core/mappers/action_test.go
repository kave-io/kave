package mappers

import (
	"testing"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/runtime"
)

func TestActionToRecord_NilReturnsNil(t *testing.T) {
	got := ActionToRecord(nil)
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestActionToRecord_PreservesIdentityFields(t *testing.T) {
	action := &runtime.Action{
		Invocation: runtime.Invocation{
			InvocationRef: runtime.InvocationRef{
				ID:        "act-123",
				RunID:     "run-456",
				AgentID:   "agent-789",
				ProjectID: "proj-111",
				EnvID:     "env-222",
				ParentID:  strPtr("parent-333"),
			},
			InvocationTarget: runtime.InvocationTarget{
				Type:      runtime.TypeLLM,
				Connector: "openai",
				Method:    "chat.completions",
			},
		},
	}

	rec := ActionToRecord(action)

	if rec.ID != "act-123" {
		t.Errorf("ID: got %q, want %q", rec.ID, "act-123")
	}
	if rec.RunID != "run-456" {
		t.Errorf("RunID: got %q, want %q", rec.RunID, "run-456")
	}
	if rec.AgentID != "agent-789" {
		t.Errorf("AgentID: got %q, want %q", rec.AgentID, "agent-789")
	}
	if rec.ProjectID != "proj-111" {
		t.Errorf("ProjectID: got %q, want %q", rec.ProjectID, "proj-111")
	}
	if rec.EnvID != "env-222" {
		t.Errorf("EnvID: got %q, want %q", rec.EnvID, "env-222")
	}
	if rec.ParentID == nil || *rec.ParentID != "parent-333" {
		t.Errorf("ParentID: got %v, want %q", rec.ParentID, "parent-333")
	}
}

func TestActionToRecord_SourceIsIntercepted(t *testing.T) {
	action := &runtime.Action{
		Invocation: runtime.Invocation{
			InvocationRef: runtime.InvocationRef{ID: "act-1", RunID: "run-1"},
		},
	}

	rec := ActionToRecord(action)

	if rec.Source != string(runtime.ActionSourceIntercepted) {
		t.Errorf("Source: got %q, want %q", rec.Source, string(runtime.ActionSourceIntercepted))
	}
}

func TestActionToRecord_OutcomeStoredInMetadata(t *testing.T) {
	action := &runtime.Action{
		Invocation: runtime.Invocation{
			InvocationRef: runtime.InvocationRef{ID: "act-1", RunID: "run-1"},
		},
		Outcome: &runtime.Outcome{
			Code:    "denied",
			Message: "rate limit exceeded",
			Reason:  "budget exhausted",
		},
	}

	rec := ActionToRecord(action)

	if rec.Metadata == nil {
		t.Fatal("Metadata is nil, expected to contain outcome fields")
	}

	if code, ok := rec.Metadata["outcome_code"].(string); !ok || code != "denied" {
		t.Errorf("outcome_code: got %v, want 'denied'", rec.Metadata["outcome_code"])
	}
	if msg, ok := rec.Metadata["outcome_message"].(string); !ok || msg != "rate limit exceeded" {
		t.Errorf("outcome_message: got %v, want 'rate limit exceeded'", rec.Metadata["outcome_message"])
	}
	if reason, ok := rec.Metadata["outcome_reason"].(string); !ok || reason != "budget exhausted" {
		t.Errorf("outcome_reason: got %v, want 'budget exhausted'", rec.Metadata["outcome_reason"])
	}
}

func TestActionToRecord_NilOutcomeProducesNilMetadata(t *testing.T) {
	action := &runtime.Action{
		Invocation: runtime.Invocation{
			InvocationRef: runtime.InvocationRef{ID: "act-1", RunID: "run-1"},
		},
		Outcome: nil,
	}

	rec := ActionToRecord(action)

	if rec.Metadata != nil {
		t.Errorf("Metadata should be nil when Outcome is nil, got %v", rec.Metadata)
	}
}

func TestObservedActionToRecord_SourceIsObserved(t *testing.T) {
	action := &runtime.ObservedAction{
		Invocation: runtime.Invocation{
			InvocationRef: runtime.InvocationRef{ID: "act-1", RunID: "run-1"},
		},
	}

	rec := ObservedActionToRecord(action)

	if rec.Source != string(runtime.ActionSourceObserved) {
		t.Errorf("Source: got %q, want %q", rec.Source, string(runtime.ActionSourceObserved))
	}
}

func TestObservedActionToRecord_NilReturnsNil(t *testing.T) {
	got := ObservedActionToRecord(nil)
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestRecordToAction_NilReturnsNil(t *testing.T) {
	got := RecordToAction(nil)
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestRecordToAction_PreservesFields(t *testing.T) {
	rec := &runtimemodel.ActionRecord{
		ID:        "act-123",
		RunID:     "run-456",
		AgentID:   "agent-789",
		ProjectID: "proj-111",
		EnvID:     "env-222",
		ParentID:  strPtr("parent-333"),

		ActionType: "llm",
		Connector:  "openai",
		Method:     "chat.completions",

		Input:  bytePtr([]byte(`{"model":"gpt-4"}`)),
		Output: bytePtr([]byte(`{"choices":[...]}`)),
		Error:  nil,

		Status: "completed",
	}

	action := RecordToAction(rec)

	if action.ID != "act-123" {
		t.Errorf("ID: got %q, want %q", action.ID, "act-123")
	}
	if action.RunID != "run-456" {
		t.Errorf("RunID: got %q, want %q", action.RunID, "run-456")
	}
	if action.Type != runtime.TypeLLM {
		t.Errorf("Type: got %v, want %v", action.Type, runtime.TypeLLM)
	}
	if action.Connector != "openai" {
		t.Errorf("Connector: got %q, want %q", action.Connector, "openai")
	}
	if action.Status != runtime.StatusCompleted {
		t.Errorf("Status: got %v, want %v", action.Status, runtime.StatusCompleted)
	}
	if action.Input == nil || string(*action.Input) != `{"model":"gpt-4"}` {
		t.Errorf("Input round-trip failed: got %v", action.Input)
	}
}

func TestRecordToAction_NoBlockedStatusFromObserved(t *testing.T) {
	// A record with Source=observed and Status=blocked can be round-tripped.
	// RecordToAction doesn't enforce that observed actions can't have blocked status.
	// The type system enforces this at the runtime.ObservedAction level, not in the mapper.
	rec := &runtimemodel.ActionRecord{
		ID:         "act-1",
		RunID:      "run-1",
		ActionType: "llm",
		Connector:  "openai",
		Method:     "chat.completions",
		Status:     "blocked", // Technically invalid for observed, but mapper allows it
		Source:     string(runtime.ActionSourceObserved),
	}

	action := RecordToAction(rec)

	if action.Status != runtime.ActionStatus("blocked") {
		t.Errorf("Status: got %v, want blocked", action.Status)
	}
	// The mapper succeeds. The runtime layer prevents constructing invalid ObservedAction
	// instances with blocked status (that's a type-level contract).
}

// --- helpers ---

func strPtr(s string) *string  { return &s }
func bytePtr(b []byte) *[]byte { return &b }
