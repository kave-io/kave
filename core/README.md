# Kave Core

Control plane runtime for AI agents.

## Package Architecture

Four-package rule:

- **`runtime/`** — live execution domain (Action, Run, Policy, Outcome)
- **`model/`** — persisted records (ActionRecord, SpanRow, BudgetEntry)
- **`ops/`** — module interfaces and decision types (auth.Decision, cost.BudgetStatus)
- **`mappers/`** — translation layer between runtime ↔ model (only)

See `CLAUDE.md` for the complete design spec, confirmed decisions, and data models.

## Building

```bash
go build ./...
go test ./...
```
