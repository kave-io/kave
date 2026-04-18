# Core Package Architecture

## Package map

```
core/
  runtime/     — Invocation, Action, ObservedAction, Run, TokenUsage, context helpers
  policy/      — Policy, AuthPolicy, CostPolicy, TracePolicy, ValidationPolicy
  pipeline/    — Result, Handler, Interceptor, Pipeline
  ports/       — connector contracts and capability descriptors
  ops/
    auth/       — PolicyEngine, Decision
    trace/      — Tracer interface, Span
    cost/       — Pricer, BudgetEvaluator, Meter, BudgetStatus
    validate/   — Validator interface, Schema, Result, Violation
  model/        — storage-facing data models (plain Go primitives only)
  pkg/
    constants/ — app-wide constants
    fp/        — Map, Filter, Reduce, etc.
    pointer/   — Ptr, Deref generics
```

## Dependency graph

```
pkg/         → nothing
runtime/     → policy/     (for context helpers)
policy/      → nothing
pipeline/    → runtime/
ports/       → runtime/, pipeline/
ops/auth/    → runtime/, policy/
ops/cost/    → policy/
ops/trace/   → nothing
ops/validate/→ policy/
model/       → nothing
```

No cycles. No god package. Runtime stays separate from execution pipeline and policy composition.
Core defines the contracts; connector implementations live outside core and import these interfaces.

---

## The three-way model

Every unit of work that flows through Kave falls into one of three categories:

| Category | Kave's role | Can block? | Pipeline | Produces |
|----------|-------------|------------|----------|----------|
| **Action** | Causal — in the request path | Yes | `Pipeline` (preventive) | Action + Span |
| **ObservedAction** | Observer — notified after the fact | No | `AuditPipeline` (detective) | ObservedAction + Span |
| **Import** | Receiver — pure telemetry | No | None | Span only |

- **Action**: patterns 1 (HTTP proxy) + 3 (protocol bridge). Kave controls the execution.
- **ObservedAction**: pattern 2 (SDK report-in). Agent fires the call, notifies Kave. Auth can audit but not block.
- **Import**: pattern 4 (OTel). No pipeline. Spans written directly to SpanStore.

This distinction is structural, not a runtime flag. "Blocked" is a valid status for an Action.
It is not a valid status for an ObservedAction. The type itself enforces this.
Decision detail belongs in `Action.Outcome`, not in `Error`.

---

## `runtime/`

The kernel. Everything else builds on this.

### Shared base

Both Action and ObservedAction carry identical data about what happened. The difference is only in their
relationship to Kave's causal role — expressed via Status.

```go
// Invocation is the shared data for any intercepted or observed execution.
// Not used directly - embedded by Action and ObservedAction.
type Invocation struct {
    InvocationRef
    InvocationTarget
    InvocationData
    InvocationTiming
}

type InvocationRef struct {
    ID          string
    RunID       string
    AgentID     string
    WorkspaceID string
    ParentID    *string
}

type InvocationTarget struct {
    Type      ActionType
    Connector string
    Method    string
}

type InvocationData struct {
    Input  []byte     // raw JSON
    Output []byte     // raw JSON
    Error  *string
}

type InvocationTiming struct {
    StartedAt timex.MS
    EndedAt   *timex.MS
    Depth     int        // 0 = root
    Seq       int        // sibling order
}
```

### Action and ObservedAction

```go
// action.go

type ActionType string   // "llm" | "tool" | "retrieval" | "mutation" | "api"
type ActionStatus string // see constants below
type ObservedActionStatus string // see constants below

// ActionStatus values — "blocked" is only valid here, not on ObservedAction
const (
    StatusPending   ActionStatus = "pending"
    StatusRunning   ActionStatus = "running"
    StatusCompleted ActionStatus = "completed"
    StatusFailed    ActionStatus = "failed"
    StatusBlocked   ActionStatus = "blocked"
)

// ObservedActionStatus values — no "blocked"
const (
    ObservedActionRunning   ObservedActionStatus = "running"
    ObservedActionCompleted ObservedActionStatus = "completed"
    ObservedActionFailed    ObservedActionStatus = "failed"
)

// Outcome carries structured decision detail so auth/validate/cost can explain
// why an action was blocked, warned, denied, retried, or otherwise altered.
type Outcome struct {
    Code    string
    Message string
    Reason  string
}

// Action: Kave controls this execution. Can be blocked.
// Used in patterns 1 (HTTP proxy) and 3 (protocol bridge).
type Action struct {
    Invocation
    Status  ActionStatus
    Outcome *Outcome
}

// ObservedAction: agent reported this execution to Kave. Cannot be blocked.
// Used in pattern 2 (SDK report-in).
type ObservedAction struct {
    Invocation
    Status ObservedActionStatus
}
```

### Run

```go
// run.go
type Run struct {
    ID          string
    WorkspaceID string
    AgentID     string
    PolicyID    *string
    Name        string
    Status      string   // "active" | "completed" | "failed"
    StartedAt   int64
    EndedAt     *int64
    Spent       money.Amount
    Error       *string
    Metadata    map[string]any
}

```

## `pipeline/`

Execution pipeline. Sync and async runners live here.

```go
// result.go
// Result is transport-agnostic — works for HTTP, MCP JSON-RPC, and streaming.
// No HTTP status codes or headers here.
type Result struct {
    Body       []byte
    TokenUsage *runtime.TokenUsage // nil for non-LLM actions
}

// interceptor.go
type Handler func(ctx context.Context, action *runtime.Action) (*Result, error)

// Interceptor is for the sync pipeline. Before() can return an error to block the action.
type Interceptor interface {
    Before(ctx context.Context, action *runtime.Action) (*runtime.Action, error)
    After(ctx context.Context, action *runtime.Action, result *Result) error
    Name() string
}

// pipeline.go
type Pipeline struct {
    interceptors []Interceptor
}

func New(interceptors ...Interceptor) *Pipeline

// Execute calls all Before hooks in order, then handler, then all After hooks in reverse.
// Any Before returning an error short-circuits — blocks execution, handler does not run.
func (p *Pipeline) Execute(ctx context.Context, action *runtime.Action, handler Handler) (*Result, error)
```

### Auditor and AuditPipeline (async — for ObservedActions)

```go
type ObservedActionHandler func(ctx context.Context, observedAction *ObservedAction) error

// Auditor is for the async pipeline. Process() violations are recorded but never block.
type Auditor interface {
    Process(ctx context.Context, observedAction *ObservedAction) error
    Name() string
}

// audit.go
type AuditPipeline struct {
    auditors []Auditor
}

func NewAudit(auditors ...Auditor) *AuditPipeline

// Run calls all auditors in sequence. Errors are collected and returned but
// do not stop the chain — a violation in one auditor doesn't skip the rest.
func (p *AuditPipeline) Run(ctx context.Context, observedAction *ObservedAction, handler ObservedActionHandler) error
```

### Context helpers

```go
// context.go
// Proxy and SDK handler inject these before running the pipeline.
// Interceptors and Auditors read from context — no coupling to calling code.

func WithPolicy(ctx context.Context, p *policy.Policy) context.Context
func PolicyFrom(ctx context.Context) *policy.Policy   // nil if not set

func WithRun(ctx context.Context, r *Run) context.Context
func RunFrom(ctx context.Context) *Run         // nil if not set
```

---

## `policy/`

Control policy composition root. The runtime loads one top-level policy and modules consume their typed sub-policies.

```go
// policy.go
type Policy struct {
    ID          string
    WorkspaceID string
    Name        string

    Auth       *AuthPolicy
    Cost       *CostPolicy
    Trace      *TracePolicy
    Validation *ValidationPolicy

    CreatedAt timex.MS
    UpdatedAt timex.MS
}

type AuthPolicy struct {
    PolicyID          string
    AllowedTypes      []string
    AllowedConnectors []string
    AllowedMethods    []string
}

type CostPolicy struct {
    PolicyID      string
    BudgetCap     *money.Amount
    BudgetPeriod  string
    BudgetBehavior string
}

type TracePolicy struct {
    PolicyID      string
    Input         bool
    Output        bool
    RetentionDays int
}

type ValidationPolicy struct {
    PolicyID  string
    Enabled   bool
    Retryable bool
    Config    map[string]any
}
```

---

## `ops/auth/`

Authorization module. Defines the contract for policy evaluation.

PolicyEngine evaluates an Action against an AuthPolicy and returns a structured
Decision. That keeps allow/deny, reason, and matched policy data together.

```go
// auth.go
type Decision struct {
    Allowed  bool
    Reason   string
    PolicyID *string
    Code     string
}

type PolicyEngine interface {
    Evaluate(ctx context.Context, action *runtime.Action, policy *policy.AuthPolicy) (*Decision, error)
}
```

PASETO tokens, scopes, sessions — all live in `server/infra/`. Auth in core is authz only.

---

## `ops/trace/`

Tracing module. Defines the Span record and write contract.

```go
// trace.go
type SpanKind string

const (
    SpanKindAction         SpanKind = "action"
    SpanKindObservedAction SpanKind = "observed_action"
    SpanKindImport         SpanKind = "import"
)

type SpanSource string

const (
    SourceIntercept  SpanSource = "intercept"
    SourceReport     SpanSource = "report"
    SourceOTELImport SpanSource = "otel_import"
)

type Span struct {
    ID           string
    WorkspaceID  string
    AgentID      string
    RunID        string
    ActionID     string            // references Action.ID or ObservedAction.ID
    ParentID     *string
    Name         string
    Kind         SpanKind
    Source       SpanSource
    Connector    string
    Model        *string
    StartedAt    int64
    EndedAt      int64
    DurationMS   int64
    InputTokens  *int
    OutputTokens *int
    Cost         *money.Amount
    Error        *string
    Tags         map[string]string
}

type Tracer interface {
    Record(ctx context.Context, span *Span) error
}
```

`Kind` classifies the span itself, and `Source` tells you how Kave knew about it.
All three patterns land in the same SpanStore.
Span persistence is lifecycle-based: open first, then close/finalize with end data.

---

## `ops/cost/`

Cost and budget module.

```go
// cost.go
type BudgetStatus struct {
    Spent     money.Amount
    Cap       *money.Amount
    Period    string
    Remaining *money.Amount  // nil if no cap
    Exceeded  bool
}

type Pricer interface {
    Cost(connector, model string, inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int) money.Amount
}

type BudgetEvaluator interface {
    Budget(spent money.Amount, policy *policy.CostPolicy) *BudgetStatus
}

type Meter interface {
    Pricer
    BudgetEvaluator
}
```

Takes primitives. Server-side interceptor extracts values from Run and Policy before calling.

---

## `ops/validate/`

Output validation module.

```go
// validate.go
type Schema struct {
    Name       string
    Definition map[string]any
}

type Violation struct {
    Path    string
    Code    string
    Message string
}

type Result struct {
    Valid      bool
    Violations []Violation
    Retryable  bool
}

type Validator interface {
    Validate(ctx context.Context, output []byte, schema *Schema, policy *policy.ValidationPolicy) (*Result, error)
}
```

---

## What does NOT live in `core/`

| Thing                     | Where it lives                      |
|---------------------------|-------------------------------------|
| ActionStore, RunStore     | `server/store/`                     |
| SpanStore                 | `server/store/`                     |
| PolicyEngine (Casbin)     | `server/infra/casbin/`              |
| PASETO tokens             | `server/infra/paseto/`              |
| Policy cache              | `server/` (in-memory, startup)      |
| Credential store          | `server/store/`                     |
| Project, Tap, Persona     | `server/` domain models             |
| Connector impls           | `connectors/` (import `core/ports`) |
| Interceptor/Auditor impls | `server/` (import core interfaces)  |

`core/` is pure Go with zero external dependencies. It defines contracts.
Everything else is wiring and implementation.

---

## Build order

Packages build in this order. ObservedAction/Auditor/AuditPipeline are deferred until
Pattern 2 (SDK report-in) lands — no speculative code.

**Now (Pattern 1+3 scope):**
1. `pkg/` — done
2. `runtime/` — Invocation, Action, Run, context helpers
3. `policy/` — Policy + typed sub-policies
4. `pipeline/` — Result, Interceptor, Handler, Pipeline
5. `ports/` — connector contracts and capability descriptors
6. `ops/auth/` — PolicyEngine + Decision
7. `ops/trace/` — Span + Tracer (include Source field now — costs nothing)
8. `ops/cost/` — BudgetStatus + Pricer + BudgetEvaluator + Meter
9. `ops/validate/` — Schema + Violation + Result + Validator

**When Pattern 2 lands (SDK report-in):**
10. `runtime/ObservedAction`, `pipeline/Auditor`, `pipeline/AuditPipeline`
11. `connectors/frameworks/` — SDK observed-action handlers

**When Pattern 4 lands (OTel import):**
12. `connectors/protocols/otel/` — OTel receiver, direct SpanStore writes
