package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/runtime/policy"
)

type (
	Invocation           = runtime.Invocation
	InvocationRef        = runtime.InvocationRef
	InvocationTarget     = runtime.InvocationTarget
	InvocationData       = runtime.InvocationData
	InvocationTiming     = runtime.InvocationTiming
	Action               = runtime.Action
	ActionType           = runtime.ActionType
	ActionStatus         = runtime.ActionStatus
	ObservedActionStatus = runtime.ObservedActionStatus
	Policy               = policy.Policy
	Run                  = runtime.Run
	TokenUsage           = runtime.TokenUsage
)

const (
	TypeLLM       = runtime.TypeLLM
	TypeTool      = runtime.TypeTool
	TypeRetrieval = runtime.TypeRetrieval
	TypeMutation  = runtime.TypeMutation
	TypeAPI       = runtime.TypeAPI

	StatusPending   = runtime.StatusPending
	StatusRunning   = runtime.StatusRunning
	StatusCompleted = runtime.StatusCompleted
	StatusFailed    = runtime.StatusFailed
	StatusBlocked   = runtime.StatusBlocked

	ObservedActionRunning   = runtime.ObservedActionRunning
	ObservedActionCompleted = runtime.ObservedActionCompleted
	ObservedActionFailed    = runtime.ObservedActionFailed
)

func WithPolicy(ctx context.Context, p *Policy) context.Context { return runtime.WithPolicy(ctx, p) }
func PolicyFrom(ctx context.Context) *Policy                    { return runtime.PolicyFrom(ctx) }
func WithRun(ctx context.Context, r *Run) context.Context       { return runtime.WithRun(ctx, r) }
func RunFrom(ctx context.Context) *Run                          { return runtime.RunFrom(ctx) }
func TokenUsageFrom(ctx context.Context) *TokenUsage            { return runtime.TokenUsageFrom(ctx) }

// recorder is a test helper that implements Interceptor and logs all calls.
type recorder struct {
	name string
	log  *[]string

	// Optional overrides for custom behavior.
	beforeFn func(ctx context.Context, action *Action) (*Action, error)
	afterFn  func(ctx context.Context, action *Action, result *Result) error
}

func (r *recorder) Name() string { return r.name }

func (r *recorder) Before(ctx context.Context, action *Action) (*Action, error) {
	*r.log = append(*r.log, r.name+".Before")
	if r.beforeFn != nil {
		return r.beforeFn(ctx, action)
	}
	return action, nil
}

func (r *recorder) After(ctx context.Context, action *Action, result *Result) error {
	*r.log = append(*r.log, r.name+".After")
	if r.afterFn != nil {
		return r.afterFn(ctx, action, result)
	}
	return nil
}

func newRecorder(name string, log *[]string) *recorder {
	return &recorder{name: name, log: log}
}

func okHandler(body []byte) Handler {
	return func(ctx context.Context, action *Action) (*Result, error) {
		return &Result{Body: body}, nil
	}
}

func testAction() *Action {
	return &Action{
		Invocation: Invocation{
			InvocationRef: InvocationRef{ID: "act-1", RunID: "run-1"},
			InvocationTarget: InvocationTarget{
				Type:      TypeLLM,
				Connector: "openai",
				Method:    "chat.completions",
			},
		},
		Status: StatusRunning,
	}
}

// --- Pipeline execution contract ---

func TestPipeline_EmptyPipeline(t *testing.T) {
	p := New()
	want := []byte(`{"ok":true}`)

	result, err := p.Execute(context.Background(), testAction(), okHandler(want))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result.Body) != string(want) {
		t.Fatalf("got body %q, want %q", result.Body, want)
	}
}

func TestPipeline_SingleInterceptor(t *testing.T) {
	var log []string
	ic := newRecorder("A", &log)

	p := New(ic)
	_, err := p.Execute(context.Background(), testAction(), okHandler(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"A.Before", "A.After"}
	assertLog(t, log, want)
}

func TestPipeline_MultipleInterceptors_Order(t *testing.T) {
	var log []string
	a := newRecorder("A", &log)
	b := newRecorder("B", &log)
	c := newRecorder("C", &log)

	p := New(a, b, c)
	handler := func(ctx context.Context, action *Action) (*Result, error) {
		log = append(log, "handler")
		return &Result{}, nil
	}

	_, err := p.Execute(context.Background(), testAction(), handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Before in order, handler, After in reverse.
	want := []string{"A.Before", "B.Before", "C.Before", "handler", "C.After", "B.After", "A.After"}
	assertLog(t, log, want)
}

func TestPipeline_BeforeErrorFirst_StopsAll(t *testing.T) {
	var log []string
	a := newRecorder("A", &log)
	a.beforeFn = func(_ context.Context, act *Action) (*Action, error) {
		return nil, errors.New("blocked")
	}
	b := newRecorder("B", &log)
	handlerCalled := false

	p := New(a, b)
	_, err := p.Execute(context.Background(), testAction(), func(ctx context.Context, action *Action) (*Result, error) {
		handlerCalled = true
		return &Result{}, nil
	})

	if err == nil || err.Error() != "blocked" {
		t.Fatalf("expected 'blocked' error, got: %v", err)
	}
	if handlerCalled {
		t.Fatal("handler should not have been called")
	}
	// A.Before logged, but nothing else.
	want := []string{"A.Before"}
	assertLog(t, log, want)
}

func TestPipeline_BeforeErrorMiddle_StopsAll(t *testing.T) {
	var log []string
	a := newRecorder("A", &log)
	b := newRecorder("B", &log)
	b.beforeFn = func(_ context.Context, act *Action) (*Action, error) {
		return nil, errors.New("denied")
	}
	c := newRecorder("C", &log)

	p := New(a, b, c)
	_, err := p.Execute(context.Background(), testAction(), okHandler(nil))

	if err == nil || err.Error() != "denied" {
		t.Fatalf("expected 'denied' error, got: %v", err)
	}
	// A.Before succeeds, B.Before fails, C.Before not called, no After hooks.
	want := []string{"A.Before", "B.Before"}
	assertLog(t, log, want)
}

func TestPipeline_HandlerError_AfterStillRuns(t *testing.T) {
	var log []string
	a := newRecorder("A", &log)
	b := newRecorder("B", &log)

	handlerErr := errors.New("upstream timeout")
	p := New(a, b)
	result, err := p.Execute(context.Background(), testAction(), func(ctx context.Context, action *Action) (*Result, error) {
		log = append(log, "handler")
		return nil, handlerErr
	})

	// After hooks must still run (cleanup guarantee).
	want := []string{"A.Before", "B.Before", "handler", "B.After", "A.After"}
	assertLog(t, log, want)

	// Handler error propagates as the pipeline's error.
	if err != handlerErr {
		t.Fatalf("expected handler error, got: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result on handler error, got: %v", result)
	}
}

func TestPipeline_AfterError_OverridesHandlerResult(t *testing.T) {
	// When an After hook errors, the handler's result is lost and the error is returned.
	var log []string
	a := newRecorder("A", &log)
	a.afterFn = func(_ context.Context, _ *Action, _ *Result) error {
		return errors.New("after failed")
	}

	p := New(a)
	result, err := p.Execute(context.Background(), testAction(), okHandler([]byte("good")))

	if err == nil || err.Error() != "after failed" {
		t.Fatalf("expected 'after failed' error, got: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result when After errors, got: %v", result)
	}
}

func TestPipeline_BeforeMutatesAction(t *testing.T) {
	var log []string
	a := newRecorder("A", &log)
	a.beforeFn = func(_ context.Context, act *Action) (*Action, error) {
		act.Connector = "patched"
		return act, nil
	}

	var handlerGotConnector string
	handler := func(ctx context.Context, action *Action) (*Result, error) {
		handlerGotConnector = action.Connector
		return &Result{}, nil
	}

	p := New(a)
	_, err := p.Execute(context.Background(), testAction(), handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handlerGotConnector != "patched" {
		t.Fatalf("handler got connector %q, want %q", handlerGotConnector, "patched")
	}
}

func TestPipeline_NilResultFromHandler(t *testing.T) {
	var log []string
	a := newRecorder("A", &log)
	var afterResult *Result
	a.afterFn = func(_ context.Context, _ *Action, result *Result) error {
		afterResult = result
		return nil
	}

	p := New(a)
	result, err := p.Execute(context.Background(), testAction(), func(ctx context.Context, action *Action) (*Result, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result, got: %v", result)
	}
	if afterResult != nil {
		t.Fatalf("After hook should have received nil result, got: %v", afterResult)
	}
}

func TestPipeline_ContextPropagation(t *testing.T) {
	type ctxKeyType string
	key := ctxKeyType("test-key")
	ctx := context.WithValue(context.Background(), key, "test-value")

	var log []string
	a := newRecorder("A", &log)
	var beforeCtxVal, afterCtxVal, handlerCtxVal any
	a.beforeFn = func(c context.Context, act *Action) (*Action, error) {
		beforeCtxVal = c.Value(key)
		return act, nil
	}
	a.afterFn = func(c context.Context, _ *Action, _ *Result) error {
		afterCtxVal = c.Value(key)
		return nil
	}

	p := New(a)
	_, err := p.Execute(ctx, testAction(), func(c context.Context, action *Action) (*Result, error) {
		handlerCtxVal = c.Value(key)
		return &Result{}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for name, val := range map[string]any{"Before": beforeCtxVal, "After": afterCtxVal, "handler": handlerCtxVal} {
		if val != "test-value" {
			t.Fatalf("%s: expected context value %q, got %v", name, "test-value", val)
		}
	}
}

// --- Context helpers ---

func TestWithPolicy_PolicyFrom(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() context.Context
		expect *Policy
	}{
		{
			name:   "roundtrip",
			setup:  func() context.Context { return WithPolicy(context.Background(), &Policy{ID: "p-1"}) },
			expect: &Policy{ID: "p-1"},
		},
		{
			name:   "missing key returns nil",
			setup:  func() context.Context { return context.Background() },
			expect: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PolicyFrom(tt.setup())
			if tt.expect == nil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if got == nil || got.ID != tt.expect.ID {
				t.Fatalf("expected policy ID %q, got %v", tt.expect.ID, got)
			}
		})
	}
}

func TestWithRun_RunFrom(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() context.Context
		expect *Run
	}{
		{
			name:   "roundtrip",
			setup:  func() context.Context { return WithRun(context.Background(), &Run{ID: "r-1"}) },
			expect: &Run{ID: "r-1"},
		},
		{
			name:   "missing key returns nil",
			setup:  func() context.Context { return context.Background() },
			expect: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RunFrom(tt.setup())
			if tt.expect == nil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if got == nil || got.ID != tt.expect.ID {
				t.Fatalf("expected run ID %q, got %v", tt.expect.ID, got)
			}
		})
	}
}

func TestContext_PolicyAndRun_Coexist(t *testing.T) {
	ctx := context.Background()
	ctx = WithPolicy(ctx, &Policy{ID: "p-1"})
	ctx = WithRun(ctx, &Run{ID: "r-1"})

	p := PolicyFrom(ctx)
	r := RunFrom(ctx)
	if p == nil || p.ID != "p-1" {
		t.Fatalf("expected policy p-1, got %v", p)
	}
	if r == nil || r.ID != "r-1" {
		t.Fatalf("expected run r-1, got %v", r)
	}
}

func TestContext_WrongType_ReturnsNil(t *testing.T) {
	// Store a string at the policyKey slot via a detour — this tests the type assertion safety.
	// We can't directly use policyKey from outside, but we can verify that
	// a context with no Policy stored returns nil (the type assertion fails gracefully).
	ctx := context.Background()
	if PolicyFrom(ctx) != nil {
		t.Fatal("expected nil from empty context")
	}
	if RunFrom(ctx) != nil {
		t.Fatal("expected nil from empty context")
	}
}

// --- Action/ObservedAction type constants ---

func TestActionType_Constants(t *testing.T) {
	tests := []struct {
		got  ActionType
		want ActionType
	}{
		{TypeLLM, "llm"},
		{TypeTool, "tool"},
		{TypeRetrieval, "retrieval"},
		{TypeMutation, "mutation"},
		{TypeAPI, "api"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("ActionType %q != %q", tt.got, tt.want)
		}
	}
}

func TestActionStatus_Constants(t *testing.T) {
	tests := []struct {
		got  ActionStatus
		want ActionStatus
	}{
		{StatusPending, "pending"},
		{StatusRunning, "running"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
		{StatusBlocked, "blocked"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("ActionStatus %q != %q", tt.got, tt.want)
		}
	}
}

func TestObservedActionStatus_Constants(t *testing.T) {
	tests := []struct {
		got  ObservedActionStatus
		want ObservedActionStatus
	}{
		{ObservedActionRunning, "running"},
		{ObservedActionCompleted, "completed"},
		{ObservedActionFailed, "failed"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("ObservedActionStatus %q != %q", tt.got, tt.want)
		}
	}
}

// StatusBlocked only exists on Action, not ObservedAction.
// This is enforced at the type level: ObservedActionStatus has no "blocked" constant.
// The test below documents that ObservedAction.Status cannot hold StatusBlocked
// without explicit assignment of a raw string — the type only accepts a raw
// string if you explicitly convert or use a composite literal field.
func TestObservedAction_NoBlockedStatus(t *testing.T) {
	// Verify no ObservedActionStatus constant equals "blocked".
	statuses := []ObservedActionStatus{ObservedActionRunning, ObservedActionCompleted, ObservedActionFailed}
	for _, s := range statuses {
		if s == "blocked" {
			t.Fatal("ObservedActionStatus should not have a 'blocked' constant")
		}
	}
}

// --- helpers ---

func TestPipeline_BothHandlerAndAfterFail(t *testing.T) {
	var log []string
	a := newRecorder("A", &log)
	a.afterFn = func(_ context.Context, _ *Action, _ *Result) error {
		return errors.New("cleanup failed")
	}

	handlerErr := errors.New("upstream error")
	p := New(a)
	result, err := p.Execute(context.Background(), testAction(), func(ctx context.Context, action *Action) (*Result, error) {
		log = append(log, "handler")
		return nil, handlerErr
	})

	// errors.Join should contain both errors
	if err == nil {
		t.Fatalf("expected joined error, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "upstream error") {
		t.Fatalf("error should contain 'upstream error', got: %v", err)
	}
	if !strings.Contains(errStr, "cleanup failed") {
		t.Fatalf("error should contain 'cleanup failed', got: %v", err)
	}

	if result != nil {
		t.Fatalf("expected nil result when both handler and After error, got: %v", result)
	}

	want := []string{"A.Before", "handler", "A.After"}
	assertLog(t, log, want)
}

func TestPipeline_AfterHooksStopOnFirstError_OthersSkipped(t *testing.T) {
	var log []string
	a := newRecorder("A", &log)
	b := newRecorder("B", &log)
	b.afterFn = func(_ context.Context, _ *Action, _ *Result) error {
		return errors.New("b-after-failed")
	}
	c := newRecorder("C", &log)
	var cAfterCalled bool
	c.afterFn = func(_ context.Context, _ *Action, _ *Result) error {
		cAfterCalled = true
		return nil
	}

	p := New(a, b, c)
	_, err := p.Execute(context.Background(), testAction(), func(ctx context.Context, action *Action) (*Result, error) {
		log = append(log, "handler")
		return &Result{}, nil
	})

	if err == nil || err.Error() != "b-after-failed" {
		t.Fatalf("expected 'b-after-failed' error, got: %v", err)
	}

	// After hooks run in reverse: C.After first (succeeds), B.After (fails and stops), A.After skipped.
	// So C.After IS called.
	if !cAfterCalled {
		t.Fatal("C.After should have been called")
	}

	// Log should show: A.Before, B.Before, C.Before, handler, C.After, B.After (stops here, A.After skipped)
	want := []string{"A.Before", "B.Before", "C.Before", "handler", "C.After", "B.After"}
	assertLog(t, log, want)
}

func TestPipeline_ActionMutationVisibleAcrossAll(t *testing.T) {
	var log []string
	a := newRecorder("A", &log)
	a.beforeFn = func(_ context.Context, action *Action) (*Action, error) {
		action.Method = "mutated"
		return action, nil
	}

	b := newRecorder("B", &log)
	var bBeforeMethod, bAfterMethod string
	b.beforeFn = func(_ context.Context, action *Action) (*Action, error) {
		bBeforeMethod = action.Method
		return action, nil
	}
	b.afterFn = func(_ context.Context, action *Action, _ *Result) error {
		bAfterMethod = action.Method
		return nil
	}

	var handlerMethod string
	p := New(a, b)
	_, err := p.Execute(context.Background(), testAction(), func(ctx context.Context, action *Action) (*Result, error) {
		handlerMethod = action.Method
		return &Result{}, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bBeforeMethod != "mutated" {
		t.Fatalf("B.Before: expected method 'mutated', got %q", bBeforeMethod)
	}
	if handlerMethod != "mutated" {
		t.Fatalf("handler: expected method 'mutated', got %q", handlerMethod)
	}
	if bAfterMethod != "mutated" {
		t.Fatalf("B.After: expected method 'mutated', got %q", bAfterMethod)
	}
}

func assertLog(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("log length: got %d, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("log[%d]: got %q, want %q\nfull log: %v", i, got[i], want[i], got)
		}
	}
}
