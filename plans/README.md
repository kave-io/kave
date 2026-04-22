# Kave — Ship-to-v1 Backlog

Plans 01–08 are merged. What remains to reach a shippable v1 + a credible cloud split.

One plan = one haiku agent session. Ordered by dependency: auth first (everything else assumes real identity), then correctness (audit, tests), then surface (dashboard, CLI fills), then cloud.

1. [09-auth-enforcement.md](09-auth-enforcement.md) — wire the auth interceptor, kill the bearer-UUID shortcut, finish vault/keyring credential resolution, reconcile the two casbin homes.
2. [10-cli-fillout.md](10-cli-fillout.md) — close the ~98 `output.NotImplemented` CLI handlers against the now-complete HTTP bridge + gRPC surface. Includes rebuilding `kave watch` over existing SSE streams.
3. [11-code-audit.md](11-code-audit.md) — duplicates, dead code, naming drift, undocumented policies, migration-numbering landmines, and doc-in-code gaps surfaced in the v1 sweep.
4. [12-dashboard-sync.md](12-dashboard-sync.md) — catch the Vue UI up to the post-`core/` data model (Org/User/Env, money.Amount, provenance, trace tree view).
5. [13-testing.md](13-testing.md) — exhaustive test strategy: unit, integration, e2e, benchmarks, and perf baselines on the hot paths (pipeline, gateway, span store, SSE fanout).
6. [14-kave-cloud.md](14-kave-cloud.md) — detailed design + data model for the hosted SaaS layer. Separate repo, consumed here.

### Workflow
User hands plan to a haiku agent. Returns. Opus reviews diffs; issues next two plans.

### Invariants every plan must preserve
- `core/pkg/ids.New(prefix)` for all IDs (never `uuid.NewString()`). Prefixes: `org, usr, mbr, prj, env, agn, pol, tok, cred, run, act, spn, bge, aud, trc, bind, role`. Trace/span ids use `ids.TraceID()` / `ids.SpanID()` (OTel hex).
- All HTTP JSON responses go through `server/internal/contract.WriteSuccess` / `WriteError`.
- Money fields: `contract.Money{Amount, Currency}` on the wire; `core/pkg/money.Amount` in-process. Never float dollars outside `PriceBook`.
- Paired time fields: `<name>` (ISO-8601) + `<name>_ms` (unix milli).
- Cursor pagination everywhere: `store.Page{Limit, Cursor}` in, `PageResult[T]{Items, NextCursor}` out.
- No breaking proto wire changes without `buf generate` + `proto/gen` update.
- `go build ./...` and `go test ./...` clean in every module before hand-off.
