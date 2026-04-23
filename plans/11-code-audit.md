# Plan 11 — Code Audit: Remaining Drift and Landmines

**Goal:** a smaller residual cleanup pass. The big subtraction sprint already landed (7 modules → 4, core/ops collapsed, duplicate casbin killed, dead `core/pkg/{fp,pointer,constants}` removed, server/db merged into server/store, BudgetEntry metadata promoted to typed fields, default identity seeded). What's left is specific, not structural.

## Read first

- `core/store/store.go` — 15+ store interfaces plus `AppStore` umbrella. Test ergonomics, not correctness.
- `server/internal/store/{sqlite,postgres,duckdb}/migrations/` — numbering inconsistency survives (see #2).
- `server/internal/store/duckdb/span_store.go` vs `server/internal/store/postgres/span_store.go` — WHERE-clause builders diverged during plan 08.

## Specific findings

### 1. Policy interceptor vs casbin split
`server/ops/policy/interceptor.go` evaluates `PolicyRecord.AllowedTypes/Connectors/Methods` allow-lists directly. `server/internal/infra/casbin/` exists but the interceptor doesn't call it. **Decision:** keep allow-list as the fast path for the default policy; route bespoke policies through casbin when a non-empty `CasbinDocument` is set on the record:

```go
if pol.CasbinDocument != "" {
    return i.casbin.Enforce(sub, obj, act)
}
return matchesAllowLists(pol, action)
```

Add `PolicyRecord.CasbinDocument string` + migration. Default rows leave it NULL → fast path. Plan 09 also touches this — coordinate so we don't re-do the same field.

### 2. Migration numbering drift (postgres has two 006s)
Postgres migrations contain `006_budgets.{up,down}.sql` AND `006_fx.{up,down}.sql`. Both are idempotent and were applied in filename-sort order, but the collision is a CI-run-order footgun. SQLite migrations skip 005/006 (jump 004 → 007).

**Decision:**
- Leave applied postgres migrations alone (never renumber applied).
- Add `MIGRATIONS.md` in each engine's `migrations/` dir documenting numbering and the 006 collision.
- Add startup check: if `schema_migrations` has two 006 rows with mismatched checksums, log a loud warning and refuse to start.
- Going forward, allocate numbers monotonically (010, 011, …) — no per-topic namespacing.

### 3. Span store WHERE builders diverged
`duckdb/span_store.go` and `postgres/span_store.go` both hand-build the same WHERE clause for `SpanFilter` (expanded in plan 08). Extract `server/internal/store/spansql/where.go`:

```go
func BuildWhere(f model.SpanFilter, d Dialect) (sql string, args []any)
```

Dialect enum handles `$1` vs `?` placeholders and JSONB-vs-TEXT metadata. Both stores collapse.

### 4. TraceID aliasing (verify)
Plan 08 was supposed to fix `server/ops/trace/postgres_tracer.go` to stop aliasing `TraceID = RunID`. Confirm `TraceID` is now set from `action.TraceID` populated by the pipeline. If not, fix here.

### 5. `kave watch` CLI stub
`cli/internal/commands/lifecycle/watch_handler.go` returns `NotImplemented`. Plan 10 rewrites it over composed SSE streams. If plan 10 is delayed, delete the stub rather than shipping a NotImplemented.

### 6. Remaining `uuid.NewString` holdout
`server/internal/store/postgres/app_store.go:710` still uses `uuid.NewString()` for price snapshot IDs. Migrate to `ids.New("psn")`. After this lands, `grep -r 'uuid.NewString' --include='*.go'` should be empty.

### 7. Store interface ergonomics
`core/store.AppStore` aggregates 15+ interfaces. Production wiring stays the same, but tests that need two methods have to satisfy 200. Add minimal fakes in `core/store/fake/` — one tiny fake per feature interface (fakeAgentStore, fakeRunStore, …) so handler tests can compose only what they need.

### 8. Connector credential encryption at rest — document threat model
`core/pkg/keyring/keyring.go` encrypts/decrypts via OS keyring. `encrypted` source path: where does the DEK live — keyring or config? Add `docs/internal/credential-encryption.md` naming the key store, the threat model, and what *isn't* covered. Plan 14 (cloud) assumes encrypted credentials are safe at rest.

### 9. Undocumented invariants
Add doc comments where missing:
- `core/pkg/money.Amount` — nano-dollars, int64, wraparound semantics, USD baseline assumption.
- `core/pkg/ids.New` — prefix registry at top of file listing every known prefix (org, usr, mbr, prj, env, agn, pol, tok, cred, run, act, spn, bge, aud, trc, bind, role, psn).
- `pipeline.Execute` — contract: Before runs in order, short-circuit on error skips upstream and remaining Before; After runs in reverse.
- `core/runtime.Action` already gained its high-level doc (Action vs Span). Extend with per-field lifecycle for `TraceID/SpanID/ParentID` (who writes, who reads, when).

### 10. Pipeline vs gRPC Interceptor name clash
`core/pipeline.Interceptor` and grpc `UnaryInterceptor` both carry "Interceptor" in the semantic space. LSP collision irritates. Rename pipeline → `core/pipeline.Stage`. Mechanical rename across all call sites.

### 11. Repo-root hygiene
- `BACKLOG.md` at repo root is obsoleted by `plans/`. Delete.
- `IDEAS.md` → move to `docs/internal/`.
- `kave.db`, `kave-spans.duckdb*` at repo root — verify `.gitignore` covers and remove any accidentally tracked DB/wal files.

### 12. `server/internal/infra/{crypto,paseto}` location
If each is <200 LOC of thin wrappers, move to `core/pkg/`. If they carry real local policy (TTLs, algorithms selected), leave them — but then add a comment saying why they're server-only. Pick one.

## Files

Per finding — no single manifest. Aim to close 8–10 of 12 in one pass.

## Acceptance

- One commit per finding (or grouped set) referencing the finding number.
- `go build ./... && go test ./...` clean after each commit.
- `MIGRATIONS.md` exists in both sqlite and postgres migrations dirs with the 006 collision explained.
- `grep -r 'uuid.NewString' --include='*.go'` returns empty.
- `rg -l "TODO|FIXME|XXX" core/ server/ cli/` list shrinks.

## Out of scope
- Further behavioral changes.
- Performance refactors (plan 13).
- Adding features.

## Size estimate
~400 LOC of consolidation + docs. One haiku session, strict top-to-bottom.
