# Plan 11 — Code Audit: Duplicates, Dead Code, Drift, Landmines

**Goal:** do a focused cleanup pass of things that accumulated across plans 01–08. Not a rewrite — a concrete list of known specific smells, each with a decision and a fix. Opus's v1 sweep flagged these; left unattended they'll bite us during cloud work.

## Read first
- `core/store/store.go` — 15+ store interfaces plus `AppStore` umbrella. Skim for coherence.
- `server/internal/db/sqlite/` and `server/internal/db/postgres/migrations/` — numbering is inconsistent (see #5).
- `server/internal/infra/casbin/` vs `server/ops/auth/casbin_engine.go` — two homes.
- `server/internal/store/duckdb/span_store.go` vs `server/internal/store/postgres/span_store.go` — WHERE-clause builders diverged during plan 08.
- `core/pkg/` — growing grab-bag: `authhash`, `constants`, `fp`, `ids`, `keyring`, `money`, `pointer`, `timex`. Check for thin wrappers that could go inline.
- `cli/internal/commands/lifecycle/lifecycle.go:21,117` — `newWatchCmd` + `watch_handler.go` still exist and return NotImplemented. Plan 10 rebuilds them; this plan removes the dead path if plan 10 hasn't shipped yet.

## Specific findings

### 1. Duplicate casbin engine
- `server/ops/auth/casbin_engine.go` and `server/internal/infra/casbin/casbin.go` both wrap casbin. Keep `internal/infra/casbin` (owns model + audit hook). Migrate any unique behavior out of `ops/auth/casbin_engine.go`, then delete it.
- **Plan 09 also touches this** — coordinate. If plan 09 ships first, this reduces to a verification check.

### 2. Policy interceptor bypasses casbin
`server/ops/policy/interceptor.go` evaluates `PolicyRecord.AllowedTypes/Connectors/Methods` allow-lists directly. That works, but it forks the authorization model: auth uses casbin, policy uses allow-lists. **Decision: keep the allow-list for fast-path default policy, but route bespoke policies through casbin.** Wire as follows:

```go
if pol.HasCasbinDocument {
    return i.casbin.Enforce(sub, obj, act)
}
return matchesAllowLists(pol, action)
```

`PolicyRecord` gains an optional `CasbinDocument string` field (raw policy lines). Migration adds the column; existing rows keep NULL → fast path.

### 3. Migration numbering drift
- SQLite: 001, 002, 003, 004, **(missing 005, 006)**, 007, 008.
- Postgres: 001, 002, 003, 004, 005_cost_model_gaps, **006_budgets + 006_fx (two 006s!)**, 007, 008.
- The deleted `server/internal/db/sqlite/005_identity.sql` never had 005/006 siblings.

**Decision:** renumber postgres `006_fx` → `006a_fx` won't work in migration tools. Instead:
- Leave existing prod postgres migrations alone (never renumber applied migrations).
- Add a `MIGRATIONS.md` next to each `migrations/` dir documenting the 006 collision and why it's benign (both up/down are idempotent and were applied in filename order).
- For SQLite, renumbering is safe *if* no one has the old DB — document "v1 = fresh sqlite DB" in upgrade notes.
- Add a startup check that logs a loud warning if the applied-migrations table shows both 006 rows with mismatched checksums.

### 4. Span store WHERE builders diverged
`duckdb/span_store.go` and `postgres/span_store.go` both hand-build the same WHERE clause for `SpanFilter`. Plan 08 expanded the filter; drift is likely. Extract a shared builder:

```go
// server/internal/store/spansql/where.go
func BuildWhere(f model.SpanFilter, dialect Dialect) (sql string, args []any)
```

Dialect handles `$1` vs `?` placeholders and JSONB-vs-TEXT metadata access. Both stores collapse to one-liners.

### 5. `TraceID = RunID` placeholder in tracer (should be fixed by plan 08; verify)
Plan 08 was supposed to fix `server/ops/trace/postgres_tracer.go` to stop aliasing `TraceID = RunID`. Audit: confirm `TraceID` is now set from `action.TraceID` (populated by pipeline) and `RootSpanID` is derived, not aliased. If not, fix here.

### 6. Vestigial `kave watch` CLI
`cli/internal/commands/lifecycle/watch_handler.go` returns `NotImplemented`. Plan 10 rewrites it over composed SSE streams. If plan 10 is delayed: delete the CLI command (and remove from `lifecycle.go` + `root.go` see_also) rather than shipping a NotImplemented in v1.

### 7. Grab-bag `core/pkg/constants`
Package exists but only holds one or two values per file-type. Either (a) fold into the package that uses the constant, or (b) commit to `core/pkg/constants` as the one home for app-wide string/int constants and document what belongs. Pick one. Current state invites drift.

### 8. `core/pkg/fp` — unused?
Functional helpers package. Verify with `grep -r "pkg/fp" --include='*.go'`. If only self-referential, delete. If used, add a 3-line doc comment naming two callers.

### 9. `core/pkg/pointer` overlap with standard library
Go 1.22+ has many pointer helpers via generics. Audit if `pkg/pointer` duplicates `&` conversions; collapse to `ptr.To[T]` and delete the rest.

### 10. BudgetEntry ID invariant (fixed — verify)
`server/ops/budget/interceptor.go:136,174` previously used `uuid.NewString()`. Fixed to `ids.New("bge")` on commit b5945bd. Confirm no regressions.

### 11. Connector credential encryption at rest
`core/pkg/keyring/keyring.go` encrypts/decrypts via OS keyring. But postgres-stored encrypted credentials use what key? Audit the `encrypted` source path: the DEK must live in keyring or a KMS, never in the DB. If it's currently in a config value — document the threat model clearly or move to keyring. **Plan 14 (cloud) assumes encrypted credentials are safe at rest; this audit is a prerequisite.**

### 12. `core/store.AppStore` is 15+ embedded interfaces
Not a bug, but hard to mock. Tests that need 2 methods have to satisfy 200. Decision: keep `AppStore` as the production-wiring aggregate, but add per-interface mocks via `mockery` or hand-written minimal fakes in `core/store/fake/`. Small interfaces at call sites are the Go idiom.

### 13. Two `Interceptor` types (pipeline vs gRPC)
`core/pipeline.Interceptor` and gRPC `UnaryInterceptor`. Different concerns, name collision irritates LSP. Rename pipeline → `core/pipeline.Stage` (it's a stage in the action lifecycle). Mechanical rename; all interceptors implement the same method set.

### 14. Undocumented invariants
Add doc comments where missing for:
- `core/pkg/money.Amount` — why nano-dollars, why int64, wraparound semantics, currency assumption (USD baseline).
- `core/pkg/ids.New` — prefix registry. Add a comment listing all known prefixes at the top of the file.
- `pipeline.Execute` — contract: interceptors run Before in order, upstream, then After in reverse. Document the guarantee that Before errors short-circuit before upstream.
- `core/runtime.Action` — each field's lifecycle (who writes, who reads, when).

### 15. `IDEAS.md` and `BACKLOG.md` at repo root
`BACKLOG.md` is obsoleted by plans/. `IDEAS.md` is scratch. Decision: move `IDEAS.md` under `docs/internal/` and delete `BACKLOG.md` (it references the deleted `kave watch` endpoint and pre-plans state anyway).

### 16. `kave.db`, `kave-spans.duckdb`, `kave-spans.duckdb.wal` checked in
They're in repo root (from `.gitignore` review). Confirm `.gitignore` covers them; delete any accidentally tracked DB/wal files.

## Files

Touch per finding — no single manifest. Fix scope is bounded: each finding above is 1–100 LOC of changes. Aim to close 8–12 of the 16 in one pass; punt the rest into a follow-up with a short explanation.

## Acceptance

- One commit per finding (or one commit per grouped set) with a message referring to the finding number.
- `go build ./... && go test ./...` clean after each commit.
- `MIGRATIONS.md` files exist for both sqlite and postgres with the 006 collision explained.
- A follow-up issue or TODO-in-README for any finding explicitly punted.
- `grep -r "uuid.NewString" --include='*.go'` returns at most the already-known callers in `server/internal/store/postgres/app_store.go:710` (price snapshot) and `server/ops/cost/meter.go:60` — both should also migrate to `ids.New`, do it here.
- `rg -l "TODO|FIXME|XXX" core/ server/ cli/ app/` list is smaller after this plan than before.

## Out of scope
- Functional/behavioral changes beyond what's listed.
- Performance refactors (plan 13).
- Adding features. This plan *removes*; it does not add.

## Size estimate
~600 LOC of deletions + 200 LOC of consolidation + docs. One haiku session if it works the list top-to-bottom and hard-stops at plan 11 — it must not become "while I'm here…"
