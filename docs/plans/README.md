# Kave — Ship-to-v1 Backlog

Plans 01–11 are merged. What remains to reach a shippable v1 + a credible cloud split.

One plan = one haiku agent session. Ordered by dependency.

**Baseline note (2026-04-23):** big subtraction sprint already shipped before these plans — 7 modules → 4 (`core`, `server`, `cli`, `proto/gen`), `core/ops/*` collapsed into `core/runtime/*`, `core/ports/` inlined, `core/pkg/{fp,pointer,constants}` deleted, `server/internal/db/*` merged into `server/internal/store/<engine>/migrations/`, `server/ops/auth/casbin_engine.go` deleted, BudgetEntry `Metadata` promoted to typed `Blocked/BlockReason/BlockPeriod`, default user + membership seeded. −445 LOC net.

**Progress note (2026-04-26):**
- ✅ **Plan 09 done** — auth interceptor wired, casbin engine constructed in main, vault credresolve implemented, paseto tokens fixed (kind="user"/"agent"), tests added.
- ✅ **Plan 10 partial** — 18/49 stub handlers wired against the existing gRPC surface (agent, agent/token, policy, credential, budget×3, price×2). 29 stubs remain, blocked on plan 15 because they need RPCs that don't exist yet (rbac, daemon, streaming, etc.).
- ✅ **Audit subsystem** end-to-end (was finding #1): SQLite + Postgres `AuditStore`, gRPC service registered, tests.
- ✅ **CLI pagination** `--all` iterates `next_cursor` automatically (was finding #5).
- ✅ **CLI UX flatten** — `kave version` and `kave apply` are top-level (was finding #6, partial).
- ✅ **Server minor fixes** — `policy.created` event (was emitting `policy.updated`), non-panicking rand helpers in `core/pkg/ids` and `server/app/control/mappers`.

## Remaining plans

1. [12-dashboard-sync.md](12-dashboard-sync.md) — catch the Vue UI up to the post-`core/` data model (Org/User/Env, money.Amount, provenance, trace tree view).
2. [13-testing.md](13-testing.md) — exhaustive test strategy: unit, integration, e2e, benchmarks, and perf baselines on the hot paths (pipeline, gateway, span store). Detailed implementation specs split into 5 work-packages under [../testing/](../testing/README.md).
3. [14-kave-cloud.md](14-kave-cloud.md) — detailed design + data model for the hosted SaaS layer. Separate repo, consumed here.

## Done

- ✅ **Plan 15 (HTTP reduction)** — control plane is gRPC-only. `server/internal/httpbridge` removed. Remaining HTTP surface (intentional): `/health`, the LLM gateway proxy (`/v1/openai|anthropic|google`, `/frameworks/*`), and `/api/v1/fx/*`.

## Suggested order

Plan 13 (testing) first — closes v1 contract. Plan 12 in parallel (dashboard work doesn't block tests). Plan 14 is independent and post-v1.

### Workflow
User hands plan to a haiku agent. Returns. Opus reviews diffs; issues next two plans.

### Invariants every plan must preserve
- `core/pkg/ids.New(prefix)` for all IDs (never `uuid.NewString()`). Prefixes: `org, usr, mbr, prj, env, agn, pol, tok, cred, run, act, spn, bge, aud, trc, bind, role`. Trace/span ids use `ids.TraceID()` / `ids.SpanID()` (OTel hex).
- All HTTP JSON responses go through `server/internal/contract.WriteSuccess` / `WriteError`. (Note: HTTP surface shrinks dramatically after plan 15 — most code stops needing this.)
- Money fields: `contract.Money{Amount, Currency}` on the wire; `core/pkg/money.Amount` in-process. Never float dollars outside `PriceBook`.
- Paired time fields: `<name>` (ISO-8601) + `<name>_ms` (unix milli).
- Cursor pagination everywhere: `store.Page{Limit, Cursor}` in, `PageResult[T]{Items, NextCursor}` out.
- No breaking proto wire changes without `buf generate` + `proto/gen` update.
- `go build ./...` and `go test ./...` clean in every module before hand-off.
