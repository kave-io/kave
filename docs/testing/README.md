# Kave Test Strategy — Specs for Implementation

These specs are the contract between the planning model and the agents that
implement the tests. Each numbered file is a single, self-contained
work-package: one agent, one PR.

The goal is **not coverage as a number** — it's correctness on the user paths,
clean error surfaces, observable failures (logs/traces), and stable hot-path
performance. Coverage is a by-product.

## Layered targets

| Layer | What it verifies | Where it lives | Speed |
|---|---|---|---|
| 1. Unit | pure logic, edge cases, error variants | `*_test.go` next to source | <30s total |
| 2. Integration | real stores + in-process server, real wire | `*_test.go` + `testutil/harness.go` | <3min total |
| 3. E2E | built `kave` binary, end-user flows | `e2e/*_test.go`, build tag `e2e` | best-effort |
| 4. Bench | hot paths, regression-gated | `*_bench_test.go` next to source | nightly |
| 5. Fuzz / property | parsers + invariants | `Fuzz*` + `testdata/fuzz/` | nightly |
| 6. Loadgen | drift, leaks, p99 | `cmd/loadgen/` | nightly |

## File map (work packages)

- [01-unit.md](01-unit.md) — `core/pkg/*`, `core/pipeline`, `core/mappers`, `server/ops/*`, `server/internal/config`, `server/internal/gateway/errors`. ~1500 LOC of tests.
- [02-integration.md](02-integration.md) — `server/internal/testutil/harness.go`, `core/store/storetest/suite.go`, gateway flows, auth round-trip, vault, apply, SSE fanout. Testcontainers for Postgres + Vault. ~1800 LOC.
- [03-bench.md](03-bench.md) — pipeline, gateway buffered/streaming, span store insert/query, trace tree, cost meter, SSE fanout, money, authhash. Baseline table format. ~400 LOC.
- [04-fuzz-property.md](04-fuzz-property.md) — `money.ParseAmount`, `mappers` round-trip, `trace.BuildTree` random DAGs, `config.Expand`. Seed corpora. ~250 LOC.
- [05-e2e-loadgen-contracts-ci.md](05-e2e-loadgen-contracts-ci.md) — three E2E binary tests, `cmd/loadgen`, golden contract files, `.github/workflows/test.yml`, `Makefile` targets. ~700 LOC.

## Pre-test cleanup (do FIRST, in this order)

Decisions D3–D9 and inconsistencies #1–5 are locked. Before any test PR
is opened:

1. **Architecture linter** — implement `cmd/lint-architecture` per
   `../design/03-architecture-lint.md`. Wire it as a `go test` so it gates
   every PR. This is the artifact that prevents the http-bridge incident
   from recurring.
2. **Cleanup PR** — bring code in line with the locked rules:
   - Delete duplicate `ErrBudgetExceeded` (keep `server/ops/budget`).
   - Move budget enforcement from gateway-inline into the pipeline stage
     (`server/ops/budget.Stage.Before`).
   - Move `/api/v1/fx/*` to the gRPC `FXService` (see `../design/01-fx.md`).
   - Move `/health` inline into `server/main.go`; delete
     `server/internal/contract`.
   - Replace `allowAnonymous` with the three-axis policy
     (`../design/02-auth-boundary.md`); remove the seeded "default" agent.
3. **Run the linter** — clean, with documented allowlist entries for the
   handful of intentional exceptions (FX scheduled refresh, streamhub
   heartbeats).
4. **Then** start spec 02 (the harness) and the rest of testing.

Running tests against the inconsistent code locks in the wrong shapes.
Order matters.

## v1 frozen decisions (these tests assume them)

Two semantic decisions were locked at the start of testing-mode (2026-04-27).
If prod code disagrees with the test, **fix prod code** — these are the
contract:

- **D1. Pipeline panic = recover at boundary, propagate as typed error.**
  `recover()` lives only in `core/pipeline.Pipeline.Execute`. Stages must not
  recover. Panic → `pipeline.StagePanicError{Stage, Value, Stack}` (stack ≤4KB),
  unwraps to `pipeline.ErrStagePanic`. After hooks of stages whose Before
  already ran still fire in reverse. Active span gets `panic=true` +
  `panic.stack` attribute. One `level=error` log line with `trace_id`,
  `span_id`, `stage`. Gateway maps to HTTP 500 `gateway.internal_error`.

- **D2. money.Amount overflow = hard error, never saturate.**
  `Add`, `Sub`, `Mul` return `(Amount, error)`; on overflow → `(0,
  money.ErrOverflow)`. `Div(0)` → `money.ErrDivideByZero`. No `MustAdd`,
  no saturating helpers. Reasoning: int64 covers ~$9.2 quintillion in
  USD nano-units; an overflow means accounting is broken — fail loudly.

- **D3–D9 + Boundaries B1–B10** — locked. See `memory/v1_decisions.md`
  (final answers) and `../design/00-boundaries.md` (architectural laws).
  Highlights that touch the test specs:
  - **D4 currencies:** USD (nano-USD) and TMN/Toman (milli-Toman). No
    silent mixing. Test fixtures in both currencies.
  - **D6 audit:** every write RPC produces an audit row, enforced by
    interceptor and a coverage test.
  - **D7 idempotency:** `Idempotency-Key` header on gateway POSTs;
    replay rules tested.
  - **D8 trace propagation:** `traceparent` accepted inbound and injected
    outbound; `Run.TraceID` aligns with OTel.
  - **#5 anonymous flow:** three-axis policy (env trust mode × provider
    `RequiresAuth` × bind scope). Seeded "default" agent removed; guest
    identity is synthetic and zero-budget.

Spec 01 (§1.1, §1.6) and spec 02 reference these directly. FX gets its
own integration tests per `../design/01-fx.md`; auth boundary tests per
`../design/02-auth-boundary.md`.

## Post-plan-15 surface

Control plane is gRPC-only. The HTTP surface that remains, by design:

- `/health` — liveness.
- LLM gateway proxy — `/v1/openai/*`, `/v1/anthropic/*`, `/v1/google/*`,
  `/frameworks/<name>/*`. Clients are HTTP-native; this can't move to gRPC.
- `/api/v1/fx/*` — currency utility (4 routes).

Everything else is gRPC over `bufconn` in tests, listening socket in prod.
The `httpbridge` tree is gone; `server/internal/contract` survives only as
the small `WriteSuccess`/`WriteError` helper used by gateway + fx.

## Hard conventions (every file)

- **Table-driven** when there is more than one case; field name `name` for the subtest.
- `t.Parallel()` on every leaf test that doesn't mutate process state (env, working dir, `time.Now`).
- Use `require` for setup preconditions; `assert` for value comparisons. (`stretchr/testify` — already in deps.)
- **No** `time.Sleep` for sync. Use channels, contexts, `eventually`-style polls with deadline.
- **No globals** between tests. Each test gets its own harness, store, IDs, temp dir.
- IDs in fixtures: build with `core/pkg/ids.New(prefix)` — never hand-rolled strings, never `uuid.NewString()`. (Plan-level invariant.)
- Money: `core/pkg/money.MustParse` in fixtures, `money.Amount` everywhere in-process.
- Fixtures that change with time: inject a `nowFunc` or use a fixed `time.Time` — never read `time.Now()` in assertions.
- Errors: assert with `errors.Is` / `errors.As`, never string-compare `err.Error()` (it's brittle and breaks on wrapping).
- Logging assertions: capture with a `bytes.Buffer` `slog.Handler`; assert on a structured field (`level`, `msg`, attribute key) — never a substring of the rendered line.

## Acceptance (matches plan 13)

- `make test-unit` <30s green.
- `make test-integration` <3min green against testcontainers.
- `make bench` reproducible within ±5% on the same host.
- `benchmarks/BASELINE.md` filled in for every named benchmark.
- `make loadgen` 500 rps × 60s: p99 <100ms, zero goroutine/FD drift, 0% upstream errors.
- Coverage floor: `core/` ≥85%, `server/internal/` ≥70%, repo ≥75%. Reported but not gated on PR — gating risks brittle tests; we gate on benchmarks instead.
- Contract golden file per HTTP endpoint that survives the plan-15 reduction.
- CI: unit + integration on every PR; bench + e2e + loadgen nightly.

## Out of scope (explicit)

Chaos (mid-stream kill, disk-full, OOM); cross-OS matrix; external load tools (k6/locust); UI E2E (covered by Vue's own suite in `dashboard/`).

## Implementation order (suggested)

1. spec-02 first — the harness unblocks every integration test and removes per-test boilerplate.
2. spec-01 in parallel — pure-unit work, no shared state.
3. spec-04 after spec-01 — fuzz reuses the same fixtures.
4. spec-03 after spec-02 — benchmarks need the harness for end-to-end paths.
5. spec-05 last — depends on everything else for the contract goldens and the CI matrix.
