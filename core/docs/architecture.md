# Core Package Architecture

## Package map

```
core/
  intercept/   — Unit, Action, Event, Run, Policy, Result, Interceptor, Auditor, Pipeline, AuditPipeline, context helpers
  auth/        — PolicyEngine interface
  trace/       — Tracer interface, Span
  cost/        — Meter interface, BudgetStatus
  validate/    — Validator interface, Schema
  pkg/
    constants/ — app-wide constants
    fp/        — Map, Filter, Reduce, etc.
    pointer/   — Ptr, Deref generics
```

## Dependency graph

```
pkg/         → nothing
intercept/   → nothing  (zero-dep kernel, not even pkg/)
auth/        → intercept/
cost/        → intercept/
trace/       → intercept/
validate/    → intercept/
```

No cycles. No god package. Every domain package imports exactly one thing: `intercept/`.

---

## The three-way model

Every unit of work that flows through Kave falls into one of three categories:

| Category | Kave's role | Can block? | Pipeline | Produces |
|----------|-------------|------------|----------|----------|
| **Action** | Causal — in the request path | Yes | `Pipeline` (preventive) | Action + Span |
| **Event** | Observer — notified after the fact | No | `AuditPipeline` (detective) | Event + Span |
| **Import** | Receiver — pure telemetry | No | None | Span only |

- **Action**: patterns 1 (HTTP proxy) + 3 (protocol bridge). Kave controls the execution.
- **Event**: pattern 2 (SDK report-in). Agent fires the call, notifies Kave. Auth can audit but not block.
- **Import**: pattern 4 (OTel). No pipeline. Spans written directly to SpanStore.

This distinction is structural, not a runtime flag. "Blocked" is a valid status for an Action.
It is not a valid status for an Event. The type itself enforces this.

---

## `intercept/`

The kernel. Everything else builds on this.

### Shared base

Both Action and Event carry identical data about what happened. The difference is only in their
relationship to Kave's causal role — expressed via Status.

```go
// unit.go
// Unit is the shared data for any intercepted or observed execution.
// Not used directly — embedded by Action and Event.
type Unit struct {
    ID        string
    RunID     string
    ParentID  *string
    Type      ActionType
    Connector string
    Method    string
    Input     []byte     // raw JSON
    Output    []byte     // raw JSON
    Error     *string
    StartedAt int64      // UnixMilli
    EndedAt   *int64
    Depth     int        // 0 = root
    Seq       int        // sibling order
}
```

### Action and Event

```go
// action.go

type ActionType   = string  // "llm" | "tool" | "retrieval" | "mutation" | "api"
type ActionStatus = string  // see constants below
type EventStatus  = string  // see constants below

// ActionStatus values — "blocked" is only valid here, not on Event
const (
    StatusPending   ActionStatus = "pending"
    StatusRunning   ActionStatus = "running"
    StatusCompleted ActionStatus = "completed"
    StatusFailed    ActionStatus = "failed"
    StatusBlocked   ActionStatus = "blocked"
)

// EventStatus values — no "blocked"
const (
    EventRunning   EventStatus = "running"
    EventCompleted EventStatus = "completed"
    EventFailed    EventStatus = "failed"
)

// Action: Kave controls this execution. Can be blocked.
// Used in patterns 1 (HTTP proxy) and 3 (protocol bridge).
type Action struct {
    Unit
    Status ActionStatus
}

// Event: agent reported this execution to Kave. Cannot be blocked.
// Used in pattern 2 (SDK report-in).
type Event struct {
    Unit
    Status EventStatus
}
```

### Run and Policy

```go
// run.go
type Run struct {
    ID        string
    ProjectID string
    TapID     string
    PersonaID *string
    PolicyID  string
    Status    string   // "active" | "completed" | "failed"
    StartedAt int64
    EndedAt   *int64
    SpentUSD  float64
    Error     *string
    Metadata  map[string]any
}

// policy.go
type Policy struct {
    ID        string
    ProjectID string
    Name      string

    AllowedTypes      []string  // ["*"] or ["llm","tool"]
    AllowedConnectors []string  // ["*"] or ["openai","stripe"]
    AllowedMethods    []string  // ["*"] or ["chat.completions"]

    BudgetCapUSD   *float64  // nil = unlimited
    BudgetPeriod   string    // "run" | "daily" | "monthly"
    BudgetBehavior string    // "block" | "warn"

    TraceInput    bool
    TraceOutput   bool
    RetentionDays int

    Validation map[string]any  // v2 placeholder

    CreatedAt int64
    UpdatedAt int64
}

// result.go
// Result is transport-agnostic — works for HTTP, MCP JSON-RPC, and streaming.
// No HTTP status codes or headers here.
type Result struct {
    Body []byte
}
```

### Interceptor and Pipeline (sync — for Actions)

```go
// interceptor.go
type Handler func(ctx context.Context, action *Action) (*Result, error)

// Interceptor is for the sync pipeline. Before() can return an error to block the action.
type Interceptor interface {
    Before(ctx context.Context, action *Action) (*Action, error)
    After(ctx context.Context, action *Action, result *Result) error
    Name() string
}

// pipeline.go
type Pipeline struct {
    interceptors []Interceptor
}

func New(interceptors ...Interceptor) *Pipeline

// Execute calls all Before hooks in order, then handler, then all After hooks in reverse.
// Any Before returning an error short-circuits — blocks execution, handler does not run.
func (p *Pipeline) Execute(ctx context.Context, action *Action, handler Handler) (*Result, error)
```

### Auditor and AuditPipeline (async — for Events)

```go
// auditor.go
type EventHandler func(ctx context.Context, event *Event) error

// Auditor is for the async pipeline. Process() violations are recorded but never block.
type Auditor interface {
    Process(ctx context.Context, event *Event) error
    Name() string
}

// audit.go
type AuditPipeline struct {
    auditors []Auditor
}

func NewAudit(auditors ...Auditor) *AuditPipeline

// Run calls all auditors in sequence. Errors are collected and returned but
// do not stop the chain — a violation in one auditor doesn't skip the rest.
func (p *AuditPipeline) Run(ctx context.Context, event *Event, handler EventHandler) error
```

### Context helpers

```go
// context.go
// Proxy and SDK handler inject these before running the pipeline.
// Interceptors and Auditors read from context — no coupling to calling code.

func WithPolicy(ctx context.Context, p *Policy) context.Context
func PolicyFrom(ctx context.Context) *Policy   // nil if not set

func WithRun(ctx context.Context, r *Run) context.Context
func RunFrom(ctx context.Context) *Run         // nil if not set
```

---

## `auth/`

Authorization module. Defines the contract for policy evaluation.

PolicyEngine takes primitives, not structs. This way the same engine evaluates
both Action and Event — the caller extracts the fields it needs.

```go
// auth.go
type PolicyEngine interface {
    Allowed(ctx context.Context, actionType intercept.ActionType, connector, method string, policy *intercept.Policy) (bool, error)
}
```

PASETO tokens, scopes, sessions — all live in `server/infra/`. Auth in core is authz only.

---

## `trace/`

Tracing module. Defines the Span record and write contract.

```go
// trace.go
type Span struct {
    ID           string
    RunID        string
    ActionID     string            // references Action.ID or Event.ID
    Connector    string
    Model        *string
    StartedAt    int64
    EndedAt      int64
    DurationMS   int64
    InputTokens  *int
    OutputTokens *int
    CostUSD      *float64
    Error        *string
    Tags         map[string]string
    Source       string            // "intercept" | "report" | "otel_import"
}

type Tracer interface {
    Record(ctx context.Context, span *Span) error
}
```

`Source` tells you how Kave knew about this execution. All three patterns land in the same SpanStore.

---

## `cost/`

Cost and budget module.

```go
// cost.go
type BudgetStatus struct {
    Spent     float64
    Cap       *float64
    Period    string
    Remaining *float64  // nil if no cap
    Exceeded  bool
}

type Meter interface {
    Cost(connector, model string, inputTokens, outputTokens int) float64
    Budget(spent float64, cap *float64, period string) *BudgetStatus
}
```

Takes primitives. Server-side interceptor extracts values from Run and Policy before calling.

---

## `validate/`

Output validation module.

```go
// validate.go
type Schema struct {
    Name       string
    Definition map[string]any
}

type Validator interface {
    Validate(ctx context.Context, output []byte, schema *Schema) error
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
| Connector impls           | `connectors/`                       |
| Interceptor/Auditor impls | `server/` (import core interfaces)  |

`core/` is pure Go with zero external dependencies. It defines contracts.
Everything else is wiring and implementation.

---

## Build order

Packages build in this order. Event/Auditor/AuditPipeline are deferred until
Pattern 2 (SDK report-in) lands — no speculative code.

**Now (Pattern 1+3 scope):**
1. `pkg/` — done
2. `intercept/` — Unit, Action, Run, Policy, Result, Interceptor, Handler, Pipeline, context helpers
3. `auth/` — PolicyEngine interface
4. `trace/` — Span + Tracer (include Source field now — costs nothing)
5. `cost/` — BudgetStatus + Meter
6. `validate/` — Schema + Validator

**When Pattern 2 lands (SDK report-in):**
7. `intercept/Event`, `intercept/Auditor`, `intercept/AuditPipeline`
8. `connectors/frameworks/` — SDK event handlers

**When Pattern 4 lands (OTel import):**
9. `connectors/protocols/otel/` — OTel receiver, direct SpanStore writes
