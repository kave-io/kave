# Spec 03 — Benchmarks

Each benchmark answers one question: **how does this hot path scale, and how
much does an average call cost?** The output of every benchmark goes into
`benchmarks/BASELINE.md`; CI compares to it (delta >15% fails).

## Conventions

- File: `*_bench_test.go` next to the source.
- Function: `func BenchmarkX(b *testing.B)`.
- Always call `b.ReportAllocs()` and `b.SetBytes(n)` where size is meaningful.
- Reset timer after setup: `b.ResetTimer()`.
- Sub-benchmarks (`b.Run("size=10", ...)`) for parametric variants.
- No I/O outside `b.StopTimer()` / `b.StartTimer()` blocks.
- Run twice locally with `-count=2 -benchtime=2s` to confirm stability.
- Append results to `benchmarks/BASELINE.md` using the format below.

## Benchmark catalogue

### B1. `BenchmarkPipelineExecute`
**File:** `core/pipeline/pipeline_bench_test.go`.
**Stages:** auth (no-op identity), policy (allow), budget (no-op), cost (no-op record), trace (in-memory span sink).
**Action:** synthetic `runtime.Action` with input/output token counts populated.
**Measure:** ns/op, allocs/op for one full Before/handler/After cycle. Sub-bench: `with_panic_recovery=on/off` if recovery is parameterized.

### B2. `BenchmarkGatewayForward_Buffered`
**File:** `server/internal/gateway/gateway_bench_test.go`.
**Setup:** harness with mock upstream returning a 4 KB JSON response synchronously. No real network.
**Loop:** send POST, fully read response.
**Sub-benches:** `body_size=1KB|16KB|256KB`. `b.SetBytes(bodySize)`.

### B3. `BenchmarkGatewayForward_Streaming`
**File:** same.
**Setup:** mock upstream emits N SSE events of 256 B with no inter-chunk delay.
**Sub-benches:** `chunks=10|100|1000`. `b.SetBytes(chunks*256)`.
**Assert in benchmark:** zero allocations per chunk after warm-up (use `b.ReportAllocs` and verify in BASELINE).

### B4. `BenchmarkSpanStore_Insert_DuckDB`
**File:** `server/internal/store/duckdb/span_store_bench_test.go`.
**Setup:** fresh duckdb temp file per sub-bench.
**Sub-benches:** `mode=single|batch100|batch10000`. For batch modes, group inserts in a transaction; report ns per span.
**Cleanup:** `b.StopTimer(); store.Close()`.

### B5. `BenchmarkSpanStore_Insert_Postgres`
**File:** `server/internal/store/postgres/span_store_bench_test.go`.
**Setup:** testcontainers Postgres reused via `sync.Once`. Same sub-bench shape as B4.
**Skip in short:** `if testing.Short() { b.Skip() }`.

### B6. `BenchmarkSpanStore_Query_WithFilter`
**File:** `server/internal/store/duckdb/span_store_bench_test.go`.
**Setup:** seed 100k spans across 10 traces, 5 agents, 3 envs, 2 days.
**Loop:** run a representative `SpanFilter` (env_id + agent_id + time-range + status) with `Page{Limit: 50}`.
**Report:** ns/op, rows scanned (if exposed), allocs/op.
**Sub-bench:** include the same query against postgres for comparison.

### B7. `BenchmarkTraceTree_Build_1k_Spans`
**File:** `server/ops/trace/tree_bench_test.go`.
**Setup:** generate a balanced tree of 1024 spans (depth ~10, branching ~2).
**Sub-benches:** `1000|10000|100000` spans.
**Loop:** `BuildTree(spans)` → discard.
**Report:** ns/op, allocs/op. Targets: ≤200 ns/span, ≤2 allocs/span on commodity x86.

### B8. `BenchmarkCostMeter_Compute`
**File:** `server/ops/cost/meter_bench_test.go`.
**Setup:** snapshot with non-zero pricing across all six categories; `CostUsage` populated.
**Loop:** `Calculate(...)` → discard.
**Report:** allocs/op should be `0` (pure arithmetic). If non-zero, file a finding.

### B9. `BenchmarkSSEFanout_100Subscribers`
**File:** `server/internal/httpbridge/streams_bench_test.go` (or current location).
**Setup:** fanout hub with 100 subscribers.
**Loop:** publish a 256-byte event; assert all 100 receive within 1ms via `Eventually`.
**Report:** ns/op (per publish), allocs/op. Sub-bench: `subs=10|100|1000`.

### B10. `BenchmarkMoneyAmount_AddMul`
**File:** `core/pkg/money/money_bench_test.go`.
**Loop:** `_ = a.Add(b).Mul(7)` (or whatever the API is).
**Report:** allocs/op `0`.

### B11. `BenchmarkAuthHash_Verify`
**File:** `core/pkg/authhash/authhash_bench_test.go`.
**Setup:** `hash, _ := HashPassword("correct horse battery staple")` — once, before timer.
**Loop:** `VerifyPassword(hash, "correct horse battery staple")`.
**Sub-bench:** `verify=correct|wrong_first_byte|wrong_last_byte`. p99 spread across these <2× (timing-safe sanity).

## `benchmarks/BASELINE.md` format

```
# Benchmark Baseline

Host: <CPU model>, <cores>, <RAM>, Linux <kernel>
Go: <go version>
Date: YYYY-MM-DD
Commit: <sha>

| Benchmark | Sub | ns/op | B/op | allocs/op | Notes |
|---|---|---:|---:|---:|---|
| BenchmarkPipelineExecute | - | 1234 | 320 | 6 | |
| BenchmarkGatewayForward_Buffered | body_size=1KB | ... | ... | ... | |
| ...
```

Numbers are advisory until measured on the dev host. The CI gate (>15%
regression fails) is implemented in spec-05's workflow.

## Implementation notes

- Use `benchstat` for comparisons (`go install golang.org/x/perf/cmd/benchstat@latest`).
- `make bench` runs `go test -bench=. -benchmem -benchtime=2s -count=5 ./... | tee benchmarks/latest.txt`.
- `make bench-compare` runs `benchstat benchmarks/BASELINE.txt benchmarks/latest.txt` (paired raw outputs in addition to the human-readable BASELINE.md).
- For postgres benchmarks, container reuse is mandatory — boot cost dominates otherwise.

## What an implementing agent should produce

- One `_bench_test.go` per row above.
- A first-pass `benchmarks/BASELINE.md` with measured numbers from the dev host.
- A first-pass `benchmarks/BASELINE.txt` with the raw `go test -bench` output for benchstat.
- `make bench` and `make bench-compare` targets in the Makefile.
