# Spec 04 — Fuzz + Property Tests

Go-native `testing.F` for fuzz; checked-in seed corpora. Property tests use
`testing/quick` or table-generation in plain `testing.T` — no third-party
property library.

## 4.1 `FuzzMoneyParseAmount`

**File:** `core/pkg/money/money_fuzz_test.go`.
**Seeds** (`testdata/fuzz/FuzzMoneyParseAmount/`):
- `"$1.23"`, `"-$0.01"`, `"USD 1.23"`, `"1.23 USD"`, `"0"`, `"$0.00"`, `"$9999999999.99"`, `""`, `"abc"`, `"$1,234.56"`, `"1.234"`, `"1.2.3"`.

**Body:**
```go
amt, err := money.ParseAmount(in)
if err != nil { return }
// Round-trip invariant
s := amt.String()
amt2, err2 := money.ParseAmount(s)
if err2 != nil { t.Fatal("re-parse of canonical form failed", s) }
if amt != amt2 { t.Fatalf("not stable: in=%q canon=%q amt=%d amt2=%d", in, s, amt, amt2) }
```

Asserts: no panic on any input; valid inputs survive round-trip.

## 4.2 `FuzzConfigExpand`

**File:** `server/internal/config/expand_fuzz_test.go`.
**Seeds:** `"${A}"`, `"$$"`, `"${A:-x}"`, `"${A:?msg}"`, `"${"`, `"${}"`, `"${A${B}}"`, `"$"`, `""`.
**Body:** `Expand(in, "fuzz", env)` — must not panic regardless of input. If err is nil, output length sane (≤ in size × 1024).

## 4.3 `FuzzTraceBuildTree`

**File:** `server/ops/trace/tree_fuzz_test.go`.
**Strategy:** generate a random DAG by sampling parent edges with seeded `math/rand` derived from the fuzz input bytes. Any panic is a failure. Then assert the **classification invariant**:
- If the generated graph has exactly one root and is a tree (acyclic, connected, no orphans by construction), `BuildTree` returns nil error.
- If we deliberately injected a cycle, an orphan, or multiple roots, `BuildTree` returns the matching error sentinel.

## 4.4 Property: mappers round-trip

**File:** `core/mappers/mappers_property_test.go`.
**Approach:** for each pair (`RunRecord`, `SpanRow`, `Agent`, `PolicyRecord`, `BudgetEntry`), define a `gen<T>(rng *rand.Rand)` that produces a fully-populated random instance with:
- non-nil pointer fields populated 50% of the time,
- string fields drawn from `rand` of varying lengths,
- money amounts drawn from `[-MaxInt64..MaxInt64]`,
- ID fields built via `ids.New(prefix)`,
- time fields `int64` unix-ms in a sane range.

For each: 1000 trials, each trial: `From(To(x)) == x` via `cmp.Diff`. Any diff fails the test with the seed printed.

Use `testing/quick.Config{MaxCount: 1000, Rand: rand.New(rand.NewSource(seed))}` and a fixed seed at top-of-file (+ env override `MAPPERS_PROPERTY_SEED`).

## 4.5 Property: `tree.BuildTree` random valid DAGs

**File:** `server/ops/trace/tree_property_test.go`.
**Generator:** build a tree of size N by, for span `i in [1..N-1]`, picking parent uniformly from `[0..i-1]`. This is a uniform random labeled tree. `BuildTree` must succeed and contain all N spans reachable from root.

Repeat for `N in {1, 2, 10, 100, 1000}`, 100 trials each. Then perturb each generated set:
- Add a duplicate span ID → expect `duplicate span` error.
- Drop a non-root span's parent (orphan) → expect `orphan` error.
- Add a back-edge from a leaf to root (cycle) → expect `cycle` error.

## Implementation notes

- Fuzz tests run in CI nightly with `go test -fuzz=Fuzz... -fuzztime=120s` per fuzz.
- Property tests run in normal `go test ./...` with the fixed seed; if they ever fail, the seed in the failure message is what reproduces it locally.
- Seed corpora live under `testdata/fuzz/<FuzzName>/` and are checked in.

## What an implementing agent should produce

- One `_fuzz_test.go` per fuzz target above with seed corpus committed.
- One `_property_test.go` per property target.
- A short README in `testdata/fuzz/README.md` explaining how to add seeds when triage finds new failures.
