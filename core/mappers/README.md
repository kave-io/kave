# Mappers — Three-World Translation Layer

Maps between three domain models:

1. **runtime/** — ephemeral, in-memory (Action, Run, Policy)
2. **model/** — persistent storage schema (ActionRecord, Run, PolicyRecord)
3. **proto/** — serialization (future; will follow the same patterns)

## Organization

- `action.go` — Action ↔ ActionRecord + Outcome mapping
- `run.go` — Run ↔ Run + BudgetCapUSD context
- `policy.go` — Policy ↔ PolicyRecord + config flattening
- `span.go` — Span ↔ SpanRow + Span -> SpanEnd
- `budget.go` — TokenUsage + pricing inputs -> BudgetEntry
- `token.go` — token issue input -> AgentToken + safe view
- `credential.go` — credential upsert input -> Credential + safe view
- `agent.go` — agent create/update inputs -> model + view
- `workspace.go` — workspace create input -> model + view
- `pricebook.go` — model pricebook ↔ app-layer pricing view
- `decision.go` — auth/unified decision ↔ runtime outcome
- `validation.go` — validation result -> runtime outcome/metadata
- `context.go` — run/policy/action -> ops.ExecutionContext builder
- `util.go` — Type conversions (timex.MS ↔ int64, money.Amount ↔ decimal string)
- `errors.go` — MappingError for failures

## Pattern

Each domain type has two functions:

```go
// XtoY converts from domain X to domain Y.
func XToY(x *X) *Y { ... }

// YToX converts from domain Y back to domain X.
func YToX(y *Y) *X { ... }
```

Nil inputs return nil outputs. No panics — return errors when validation is needed.

## Usage in Handlers

Instead of:
```go
// ❌ Scattered conversions
record := &model.ActionRecord{
    ID: action.ID,
    RunID: action.RunID,
    // ... 20 more manual fields
}
```

Use:
```go
// ✅ Single call
record := mappers.ActionToRecord(action)
```

## Type Conversions

Automatic conversions for:
- `timex.MS` ↔ `int64` (milliseconds since epoch)
- `money.Amount` ↔ decimal string (canonical accounting values)
- `runtime.ActionStatus` ↔ `string`
- `runtime.RunStatus` ↔ `string`
- `[]byte` ↔ `*[]byte` (nil-safe)

## Future: Proto Mappings

When proto is added:

```
proto_to_runtime.go — Message → runtime types
proto_to_model.go   — Message → model types
```

Same pattern, same organization.
