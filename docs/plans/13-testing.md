# Plan 13 — Testing: Unit, Integration, E2E, Benchmarks, Perf Baselines

**Goal:** Kave currently has scattered unit tests. Before we ship v1 and split kave-cloud into a separate repo, establish a comprehensive test matrix that covers logic correctness, edge cases, the hot paths under load, and contract stability. One place, one plan, one go at it.

## Read first

- Existing test files: `grep -rn "func Test" --include='*_test.go' | wc -l` — count baseline. Expand from there.
- `server/ops/policy/interceptor_test.go`, `server/ops/trace/tree_test.go` — shape to imitate.
- `server/internal/gateway/gateway_test.go`, `routes_test.go` — HTTP integration pattern.
- `core/pkg/money/money_test.go` — pure-unit example.
- `compose.buf.yaml`, `go.work` — how modules interrelate; tests must cross modules cleanly.
- Hot-path candidates based on the architecture:
  - `core/pipeline/pipeline.go` — every request passes through.
  - `server/internal/gateway/routes.go` — streaming forwarder (SSE tee, gzip).
  - `server/internal/store/duckdb/span_store.go` + `postgres/span_store.go` — high write volume.
  - `server/internal/gateway/routes.go` — SSE forwarding and gateway routing.
  - `server/ops/cost/service.go` — cost calculation, called per action.
  - `server/ops/trace/tree.go` — O(n) spans per trace, called per view.

## Design

### Layer 1 — Unit tests (pure functions, zero IO)

Targets and mandatory coverage:

| Package | Required coverage |
|---|---|
| `core/pkg/money` | arithmetic, rounding, JSON/text marshal, overflow, negative amounts, non-USD (if any) |
| `core/pkg/ids` | prefix registry, ULID monotonic, uniqueness under tight loops |
| `core/pkg/keyring` | encrypt/decrypt round-trip, wrong-key failure, empty input |
| `core/pkg/authhash` | same-input determinism, cost parameters, timing-safe compare |
| `core/pkg/timex` | everything exported |
| `core/pipeline` | order of Before stages, short-circuit on error, After in reverse, panic recovery |
| `core/mappers` | runtime ↔ model ↔ proto round-trips for every model |
| `server/ops/cost` | token categories, cache-read vs cache-write pricing, missing price → zero cost (no crash) |
| `server/ops/trace/tree` | single root, multiple roots (error), cycles (error), stable child order by `started_at`, orphan detection |
| `server/ops/trace/export/*` | each formatter against a known 5-span fixture → golden file |
| `server/internal/config/layered` | 5-layer merge precedence, `${VAR:-default}` expansion, list-replace semantics, invalid YAML error |
| `server/internal/gateway/errors` | `mapError` returns expected `(status, code)` for each sentinel |

Rule: every exported function in `core/pkg/*` gets at least one test. Goal: >85% line coverage in `core/`, >70% in `server/internal/`, >60% overall.

### Layer 2 — Integration tests (real stores, in-process server)

One `_test.go` file per integration boundary. Use `testcontainers-go` for postgres, `:memory:` for sqlite, temp file for duckdb, `httptest.Server` for HTTP.

Targets:

1. **Gateway happy path** — `POST /v1/openai/chat/completions` → mock upstream → response forwarded + usage parsed + span persisted + budget entry written. Assert on all four.
2. **Gateway streaming** — SSE upstream, client receives chunks in order, span closed after last chunk, cost computed from accumulated usage.
3. **Gateway policy block** — casbin denies → 403 `gateway.policy_blocked`, upstream never called (mock request counter = 0), audit row written.
4. **Gateway budget block** — budget exceeded → 402, zero-cost audit row still written.
5. **Auth round-trip** — login → session PASETO → use bearer → identity resolves → policy enforced.
6. **Vault credential resolve** — against `hashicorp/vault:latest` container; asserts upstream request carries real key.
7. **Apply engine** — apply a `kave.yaml` that creates 3 agents + 2 policies, then diff against live state = empty.
8. **Trace tree** — run a synthetic nested-action sequence and query the trace tree through the current runtime surface.
9. **SSE forwarding** — streaming gateway clients receive ordered upstream chunks and close cleanly.
10. **Cross-store parity** — run the span-store test suite against both duckdb and postgres (shared table test from `core/store/storetest/`).

Integration test harness: `server/internal/testutil/harness.go` — boots an in-process server with in-memory sqlite + real duckdb + mock upstream. Each test calls `harness.New(t)` and gets `*http.Client`, `*grpc.ClientConn`, store handles.

### Layer 3 — E2E tests (built binary)

Minimal set, slow but real:
1. `kave start` → dashboard responds 200 → `kave stop` cleanly.
2. `kave init && kave apply kave.yaml && kave agent list` shows expected rows.
3. `kave watch` receives a synthetic event within 2s.

Place under `e2e/` at repo root. Gate with `//go:build e2e` so the main test run stays fast.

### Layer 4 — Benchmarks

`*_bench_test.go` next to the file under test. Hot paths:

```go
func BenchmarkPipelineExecute(b *testing.B)          // full stack: auth+policy+budget+trace
func BenchmarkGatewayForward_Buffered(b *testing.B)  // no streaming
func BenchmarkGatewayForward_Streaming(b *testing.B) // SSE tee
func BenchmarkSpanStore_Insert_DuckDB(b *testing.B)  // single, 100-batch, 10k-batch
func BenchmarkSpanStore_Insert_Postgres(b *testing.B)
func BenchmarkSpanStore_Query_WithFilter(b *testing.B) // the plan-08 filter set
func BenchmarkTraceTree_Build_1k_Spans(b *testing.B)
func BenchmarkCostMeter_Compute(b *testing.B)
func BenchmarkSSEFanout_100Subscribers(b *testing.B)
func BenchmarkMoneyAmount_AddMul(b *testing.B)
func BenchmarkAuthHash_Verify(b *testing.B)
```

Baseline table committed at `benchmarks/BASELINE.md` after running on the dev machine (RTX 4060 Ti host — CPU is what matters here). Regressions >15% fail CI.

### Layer 5 — Property + fuzz tests

Targets that benefit:
- `core/pkg/money.ParseAmount` — fuzz with random strings, assert no panic and round-trip for valid inputs.
- `core/mappers` — property test: `ToProto(FromProto(x)) == x` for generated `RunRecord`, `SpanRow`, `Agent`, `PolicyRecord`.
- `server/ops/trace/tree.BuildTree` — generate random DAGs; valid trees build, cycles error, orphans error.
- `server/internal/config/expand` — fuzz `${VAR}` expansion for unclosed braces, escape sequences.

Go native `testing.F` for fuzzing. Checked-in seed corpus.

### Layer 6 — Perf + correctness under load

A small harness `cmd/loadgen/main.go`:
- `loadgen gateway --rps 500 --duration 60s` — sends chat completions against the gateway with a mock upstream.
- Measures: p50/p95/p99 latency, error rate, CPU, goroutine count drift, file-descriptor drift.
- Asserts: no goroutine leak (goroutine count returns to ±50 of start after 10s idle), FD count stable, memory growth <50MB over the run.

Run in CI nightly (not per-PR). Archive results to `benchmarks/history/<date>.json`.

### Contract stability

Golden file for every intentionally public HTTP endpoint's happy-path response JSON, under `server/internal/gateway/testdata/contracts/<endpoint>.json`. A single `contract_test.go` walks the table. Changing a shape requires regenerating the golden file — that diff is the contract review.

For proto: `buf breaking --against origin/main` in CI.

## Files

Create:
- `core/store/storetest/suite.go` — shared store test suite.
- `server/internal/testutil/harness.go` — integration harness.
- `benchmarks/BASELINE.md`, `benchmarks/history/.gitkeep`.
- `cmd/loadgen/main.go`.
- `e2e/*_test.go` (3 tests).
- Fuzz corpora: `core/pkg/money/testdata/fuzz/`, similar for others.
- `server/internal/gateway/testdata/contracts/*.json` — golden contract files.
- `.github/workflows/test.yml` — unit+integration on push, bench+e2e+loadgen nightly.

Modify:
- Add `_test.go` files across listed packages.
- `Makefile` — targets `test-unit`, `test-integration`, `test-e2e`, `bench`, `loadgen`, `contracts`.

## Acceptance

- `make test-unit` runs in <30s, all green.
- `make test-integration` runs in <3min against testcontainers, all green.
- `make bench` produces a stable run (rerun within 5% on the same machine).
- `BASELINE.md` documents numbers for every named benchmark.
- `make loadgen` at 500 rps for 60s: p99 <100ms, zero goroutine/FD leaks, upstream error rate 0%.
- Coverage: core ≥85%, server/internal ≥70%, overall ≥75%.
- CI runs unit+integration on every PR; nightly for bench/e2e/loadgen.
- Contract golden files exist for every bridge endpoint.

## Out of scope
- Full chaos testing (kill connections mid-stream, disk full, OOM).
- Cross-OS matrix (test Linux only; macOS/Windows CI post-v1).
- Locust / k6 external tooling — our loadgen is Go-native and sufficient.

## Size estimate
~4000 LOC of tests + 500 LOC of harness/loadgen. Two haiku sessions, split roughly: session A = unit + integration; session B = benchmarks + loadgen + contracts + CI.
