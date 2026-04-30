# Spec 01 — Unit Tests

Pure-logic tests. Zero IO. Each subsection lists: target file, test file to
create/extend, and a numbered case list. Implementer should produce one
table-driven test per logical group; the case list is the table.

Conventions: see [README.md](README.md). Every leaf test `t.Parallel()` unless
noted.

---

## 1.1 `core/pkg/money` — extend `money_test.go`

> **Decision D2 (locked AND verified in code 2026-04-27):** `Add`/`Sub`/`Mul`/`Div`/`MulRatio` already return `(Amount, error)` with `ErrOverflow`/`ErrDivisionByZero`. Existing tests at `money_test.go:138-150` cover the boundary cases. No `MustAdd` exists. Below is the **gap-fill** beyond what's already there.

**Already covered (do not re-add):** basic parse / format / JSON, MaxInt64/MinInt64 boundaries, Div(0), MulRatio div-by-zero. **Add:**

1. **`errors.Is` semantics through wrapping.** Wrap `ErrOverflow` with `fmt.Errorf("%w", err)`; assert `errors.Is(wrapped, ErrOverflow)` true. Same for `ErrDivisionByZero`. Guards future refactors that wrap.
2. **Negative-amount arithmetic.** `Amount(-100).Add(50)` → `-50`; `Amount(-100).Sub(50)` → `-150`; `Amount(-1).Mul(-1)` → `1`. (Negative paths today only have edge tests, not the happy path.)
3. **MulRatio rounding modes** for non-trivial fractions: `Amount(100).MulRatio(1, 3, RoundDown|RoundUp|RoundHalfUp)` → 33 / 34 / 33. With negative inputs.
4. **Toman parsing/formatting (D4).** Add once D4 is implemented:
   - `Parse("60000", IRT)` returns Money with Amount in milli-Toman base units; round-trip `m.String()` formats with no decimal separator.
   - `Money.Add` rejects `IRT + USD` with `ErrCurrencyMismatch`.
5. **Regression guard:** scan `money.go` for any function name matching `Must(Add|Sub|Mul|Div)`; fail if present. (Keeps the no-saturation promise durable.)
2. **Negative amounts**
   - `MustParse("-$1.23")`, `MustParse("-1.23 USD")`, JSON marshal of `-1` cent → `"-$0.01"`.
3. **Rounding**
   - `Mul` then format never loses the half-cent: build a price (`$0.0000015`-equivalent micro-units) × 1_000_000 tokens, round-half-to-even at format time only, micro-units preserved internally.
4. **JSON / text round-trip**
   - For each of: `0`, `1`, `-1`, `MaxInt64`, `MinInt64`, fractional cent: `MarshalJSON` → `UnmarshalJSON` returns the same `Amount`.
   - Reject malformed JSON: `null`, `""`, `{}`, `"abc"`, `"$"`, `"USD 1.23 EUR"` — `UnmarshalJSON` returns error, value untouched.
5. **CurrencyCode**
   - Parse case-insensitive (`"usd"`, `"USD"`, `"Usd"` → `USD`).
   - Reject unknown / 4-letter codes.
6. **Cross-currency**
   - `Amount.AddIn(other, fxRate)` with mismatched currencies returns error or applies the rate; pin behaviour with explicit cases.

**Coverage target:** every exported func + every error branch. Run `go test -cover ./core/pkg/money/...` and confirm ≥95%.

## 1.2 `core/pkg/ids` — new file `ids_test.go`

1. **Prefix preserved:** `New("agn")` → `regexp ^agn_[0-9A-HJKMNP-TV-Z]{26}$`.
2. **Empty prefix:** `New("")` → bare ULID, no underscore.
3. **Lexicographic monotonicity:** generate 10_000 IDs in a tight loop with a fixed prefix; sort ascending — sorted order equals insertion order. (ULID monotonic property.)
4. **Uniqueness under contention:** spawn 8 goroutines × 10_000 IDs into a `sync.Map`; final size = 80_000.
5. **Trace/Span hex format:** `TraceID()` returns 32 lowercase hex chars; `SpanID()` returns 16. Run 1_000 of each, none equal, none uppercase.
6. **Crypto error path:** swap `rand.Reader` with a stub that returns `io.EOF` (use a package-level variable + `t.Cleanup` to restore). Both `TraceID` and `SpanID` return wrapped error.

## 1.3 `core/pkg/keyring` — new file `keyring_test.go`

The OS keyring is unavailable in CI. Drive the **plaintext fallback** path explicitly.

1. **Set / Get / Delete round-trip** (with `KAVE_KEYRING_DISABLED=1` and `KAVE_ALLOW_PLAINTEXT_KEY=1`, `HOME=t.TempDir()`):
   - Set("svc","acct","secret") → Get returns "secret".
   - Delete → Get returns error.
2. **GetOrCreateMasterKey:** first call generates 32 bytes, second call returns the same bytes. Verify file at `$HOME/.kave/master.key` exists with mode `0600`.
3. **Permissions:** `~/.kave/keyring/<file>` written with mode `0600`; `~/.kave` dir mode `0700`.
4. **Concurrent GetOrCreate:** 16 goroutines call `GetOrCreateMasterKey`; final on-disk key exists once, all goroutines saw 32-byte non-empty result. (Race detector must pass.)
5. **Disabled fallback:** with `KAVE_ALLOW_PLAINTEXT_KEY` unset, plaintext fallback returns wrapped `ErrKeyringUnavailable`.
6. **Path encoding:** secrets named `"a/b\x00c"` and `"\x00"` survive the base64 encoding round-trip — read returns the exact bytes.

> **Implementation note for harness:** add a `t.Helper()` that sets the env vars and a temp `HOME`, returns a cleanup. Reuse across tests.

## 1.4 `core/pkg/authhash` — new file `authhash_test.go`

1. **HashPassword/Verify round-trip:** 5 random passwords (incl. empty, 1-byte, 4096-byte, multi-byte UTF-8, NUL-embedded). Verify returns true; verify with mutated password returns false.
2. **Determinism / non-determinism:** two `HashPassword("x")` calls produce **different** hashes (random salt). Verify still passes for both.
3. **Wrong-length hash:** Verify with `len(hash) != expected` returns false (no panic). Cases: `nil`, `[]byte{}`, hash with 1 byte truncated, 1 byte appended.
4. **Tamper resistance:** flip 1 bit of the salt or the hash — Verify returns false.
5. **Timing-safe compare:** smoke-test by asserting the function path goes through `subtleCompare`. (Optional: micro-bench p99 spread <2× across 1000 verifies of a wrong-by-first-byte vs wrong-by-last-byte input, with `b.ReportAllocs`.)
6. **HashToken determinism:** `HashToken("x")` is constant across 1000 calls; differs from `HashToken("y")`. Returns lowercase hex of length 64.
7. **GenerateToken:**
   - Returns `prefix + 43-char base64url`. (32-byte raw → 43 chars unpadded.)
   - `HashToken(plain)` equals returned hash.
   - Two calls produce different `plain` values.
8. **Crypto error path:** stub `rand.Reader` to return error; `HashPassword` and `GenerateToken` propagate it (wrapped, `errors.Is` against the underlying).

## 1.5 `core/pkg/timex` — extend `timex_test.go`

1. `Now()` returns unix-ms; differs by ≤2 between two consecutive calls separated by `time.Sleep(1*time.Millisecond)`.
2. Round-trip every exported helper (`FromUnixMilli`, `ToISO`, etc.) for: `0`, current, `MaxInt64`, negative.
3. Timezone correctness: ISO formatting always emits UTC (`Z` suffix).

## 1.6 `core/pipeline` — extend `pipeline_test.go`

> **Decision D1 (locked):** `recover()` lives only in `Pipeline.Execute`. Panics convert to `pipeline.StagePanicError{Stage, Value, Stack}` (stack ≤4KB), unwrap to `pipeline.ErrStagePanic`. After hooks of stages whose `Before` already ran still fire in reverse. Span gets `panic=true` + truncated stack. One `level=error` log line with `trace_id`/`span_id`/`stage`. See `v1_decisions.md`.

Most cases exist (the file is 613 LOC). Audit and **add** the gaps:

1. **Parent linkage from context:** ctx with `WithTrace(traceID, spanID)` → action emerges with `ParentID == spanID`, `InvocationRef.ParentID` set. Verify present; otherwise add.
2. **Action carries pre-set IDs:** if `action.TraceID` and `action.SpanID` are populated, pipeline does **not** overwrite them.
3. **`ids.TraceID` failure:** stub `ids` randomness to fail. Pipeline returns the error before any stage `Before` runs.
4. **`Before` panic (D1):** stage 2 of 3 panics in `Before`. Assertions:
   - Returned error: `errors.Is(err, pipeline.ErrStagePanic)`; `errors.As(err, &spe)` yields `spe.Stage == "stage2"`, `spe.Value` matches the panic value, `len(spe.Stack) > 0 && len(spe.Stack) <= 4096`.
   - Stage 1's `After` ran exactly once; stage 2's and stage 3's `After` did not run (their `Before` did not complete).
   - Handler did not run.
   - Span has attribute `panic=true` and a `panic.stack` attribute equal to the truncated stack.
   - Captured slog handler has exactly one `level=ERROR` record with attrs `trace_id`, `span_id`, `stage="stage2"`.
5. **`After` panic (D1):** stage 2 of 3 panics in `After`. Assertions:
   - All three `Before` hooks ran; handler ran and returned `(result, nil)`.
   - Stage 3's `After` ran (reverse order, before the panic), stage 2 panicked, stage 1's `After` still ran (cleanup is reliable).
   - Returned `(result, err)` — result is the handler's result; `errors.Is(err, pipeline.ErrStagePanic)` and `spe.Stage == "stage2"`.
6. **`Before` panic *and* later handler error are joined:** craft a sequence where stage 1 `Before` succeeds, stage 2 `Before` panics. The error must unwrap to `ErrStagePanic`; handler error not present (handler never ran).
7. **`Before` succeeds, handler errors, `After` panics:** result returned; error is `errors.Join(handlerErr, stagePanicErr)`. Both `errors.Is` checks pass.
8. **`After` order is reverse of `Before`:** record stage names; assert exact sequence including reverse on After.
9. **Handler error joins with After error (no panic):** `errors.Join(errH, errA)` — both `errors.Is` true.
10. **Empty stages:** `New().Execute(ctx, action, h)` runs handler exactly once, no panic recovery path exercised.
11. **Nil action:** returns error, does not panic.
12. **Context cancellation:** ctx cancelled before Execute → pipeline returns `ctx.Err()`. Cancelled between Before and handler — pin actual behaviour (document in the test). Cancelled mid-handler — handler observes; After still runs.
13. **Concurrent Execute:** 100 goroutines × 100 iterations against the same `*Pipeline` — race detector clean, no shared state mutation.

## 1.7 `core/mappers` — already 6 test files; tighten to a checklist

For **every** mapper pair (`runtime↔model`, `model↔proto`):
1. Round-trip with a fully-populated value: `From(To(x)) == x`.
2. Round-trip with a zero value.
3. Round-trip with optional/nil fields explicitly set to nil.
4. Slice round-trip preserves order and length.
5. Money fields preserve cents exactly across round-trip.

Test file template: one table-driven test per mapper, `cases` field has the
five rows above. Asserts via `cmp.Diff` with `cmpopts.EquateEmpty`.

> Property tests (Layer 5) add randomized inputs — see spec-04.

## 1.8 `server/ops/cost` — new file `meter_test.go`

`Calculate` is the hot path. `Service.Snapshot` resolves prices.

1. **All six categories priced** (input, output, cache_read, cache_write, reasoning, audio_in/out, image_units, request_count, compute_ms, storage_bytes, bandwidth_bytes): supply a snapshot with non-zero `*PerMillion` for each one in turn; assert returned `Amount` equals the expected micro-unit math (compute by hand in the test).
2. **Cache-read vs cache-write differ:** snapshot with `CacheRead = $1/M`, `CacheWrite = $5/M`; usage of `1_000_000` each → `Amount = $6.00`.
3. **Missing price:** `Calculate(nil, ...)` returns `0` (no panic). Same when snapshot has all zeros.
4. **Negative usage** (defensive): treat as 0 or surface error — pin actual behaviour.
5. **`Service.Snapshot`:**
   - Empty book → returns nil.
   - Wildcard match (`Match: ""`) returns first wildcard for that provider.
   - Substring match: model `"gpt-4o-mini"` against `Match: "gpt-4o"` matches; against `Match: "gpt-3.5"` does not.
   - Case-insensitive provider and model.
   - Provider scope: entry with no provider matches any provider; entry with `provider: "openai"` does not match `model: anthropic.claude`.
6. **Concurrent `Snapshot` + `Replace`:** 16 readers + 1 writer for 1s — race detector clean, every reader returns either old or new snapshot, never partial.
7. **`Record`:**
   - With nil usage → no DB write (use a fake `AppStore`).
   - With usage but no `RunID` → `InsertBudgetEntry` called once, `AddRunSpend` not called.
   - With usage and RunID → both called; entry's `Cost` matches `Calculate` output; `PriceVersion` and `PriceSnapshot` populated when snapshot non-nil.
   - `InsertBudgetEntry` returns error → `Record` returns it; `AddRunSpend` not called.
8. **`CheckBudget`:**
   - Agent missing → returns error.
   - Agent has `MonthlyBudget = nil` → `BudgetStatus.Cap == nil`, `Exceeded == false`.
   - Sum of spend = cap → `Exceeded == true` (boundary).
   - Sum > cap → `Remaining` is negative `Amount`. (Confirm sub-zero allowed by `money.Amount.Sub`.)

## 1.9 `server/ops/trace` — extend `tree_test.go` (currently only 43 LOC)

1. **Single root, 3 children, sorted by `StartedAt`:** insertion order shuffled, output `Children[0..2]` in started_at order.
2. **Tied `StartedAt`:** secondary sort by `Span.ID`, deterministic.
3. **Multiple roots → error** containing both root IDs.
4. **Cycle (A→B→A) → error** "cycle at".
5. **Orphan (parent not in set) → error** "orphan span".
6. **Disconnected components without cycle:** root + isolated subtree → `disconnected spans` error.
7. **Duplicate span ID → error**.
8. **Nil span / empty ID → error**.
9. **Empty input → error** "no spans".
10. **Deep tree (1000 deep, single chain):** builds without stack overflow. (Sanity for the recursive `visit`.)
11. **Wide tree (1 root, 10_000 leaves):** builds; children sorted; all reachable.

## 1.10 `server/ops/trace/export` — new file `export_test.go`

A 5-span fixture (root → 2 children → 1 grandchild under each child) shared by all formatters.

For each of `dot`, `jsonl`, `mermaid`, `otlp`, `parquet`:
1. **Golden file** at `server/ops/trace/export/testdata/golden/<format>.<ext>`.
2. Format the fixture; compare bytes to golden. `-update` flag rewrites goldens (idiomatic `flag.Bool("update", ...)`).
3. Empty tree → empty output (or documented error, pin it).
4. **Determinism:** running formatter twice yields byte-identical output. (Map ordering in `dot`/`mermaid` must be sorted.)

For `parquet`: golden is a parquet file; compare via a parquet reader rather than bytes (page header timestamps may differ). Assert row count, column schema, and per-column values.

## 1.11 `server/internal/config` — extend `expand_test.go`, `layered_test.go`

`expand_test.go` add:
1. `${VAR}` unset → `ExpandError` with line number.
2. `${VAR:-default}` unset → `default`; set+empty → `default`; set+non-empty → value.
3. `${VAR:?msg}` unset → error containing `msg`; set+empty → error.
4. `$$` literal escapes to `$`.
5. Unclosed `${` → leaves literal `$` and continues parsing rest of the line (current behaviour — pin it).
6. Empty name `${}` → ExpandError.
7. Multiple expansions per line: `${A}-${B}-$$-${C:-z}` with A=1,B=2,C unset → `1-2-$-z`.
8. Multi-line input: each line tracked separately; line number in error matches.
9. Nested-looking but flat `${A${B}}` — assert exact behaviour (likely treats inner literal).

`layered_test.go` add:
1. **5-layer precedence** (defaults < etc < home < project < env): build a fixture in each layer setting the same key, assert the highest layer wins.
2. **Map merge:** layers add keys; result has the union with later layers overriding individual keys.
3. **List replace** (not merge): later layer's list fully replaces earlier — explicit assertion on this semantic.
4. **Invalid YAML** in any layer → error wraps the layer path.
5. **Missing optional layer** (e.g., no `~/.kave/config.yaml`) → no error; processing continues.
6. **`${VAR}` expansion** runs after merge, before unmarshal; an env var referenced by a default-layer key resolves at load time.
7. **Symlink loop / unreadable file** → wrapped error with path. (Use `t.TempDir()` + `os.Chmod 0o000`, skip on root.)

## 1.12 `server/internal/gateway/errors` — new file `errors_test.go`

1. Table for `mapError`: every sentinel above (`ErrPolicyBlocked`, `ErrBudgetExceeded`, `ErrQuotaExceeded`, `ErrUnauthenticated`, `ErrUnauthorized`, `ErrProviderNotFound`, `ErrUpstream`, `default`) + the typed wrappers (`PolicyBlockedError`, `BudgetExceededError`, `serverpolicy.BlockedError`, `serverbudget.ExceededError`).
2. For each row assert `(status, code)` and the `details` map shape.
3. Nested: `errors.Join(ErrUpstream, ctx.DeadlineExceeded)` → still routes to upstream branch (`errors.Is` works through Join).
4. `*PolicyBlockedError` with empty fields: `details` is `nil` (empty map collapsed). Confirm `contractDetails(nil)` returns `nil`.

---

## What an implementing agent should produce per file

- The `*_test.go` file with the cases above as table rows.
- Add `testdata/` fixtures alongside if needed.
- Run locally: `go test -race -count=2 ./<package>/...` — must be green twice.
- Run `go test -cover ./<package>/...` and report the number in the PR description.
- No new exported APIs in production code unless a test seam genuinely needs one — and then justify in the PR.
