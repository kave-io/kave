# Spec 05 — E2E, Loadgen, Contracts, CI

## 5.1 E2E (built `kave` binary)

`//go:build e2e` on every file. Run with `make test-e2e` which sets `GOFLAGS=-tags=e2e`.

**Helper:** `e2e/internal/run.go`:
- `Build(t)` — compiles the CLI to a temp dir, returns binary path. Reuses across the package via `sync.Once`.
- `Start(t, args...) *Cmd` — spawns the process, returns a struct with stdout/stderr capture, `WaitReady(deadline)` polling `/healthz`, `Stop()` sends SIGTERM and asserts clean exit (`exit code 0`, no goroutines leaked in stderr).
- `MockUpstream(t)` — same `MockUpstream` helper from `server/internal/testutil`, exported via a small re-export.

**Test 1 — `e2e/start_stop_test.go`**

`TestStartStop_DashboardResponds`:
1. `kave start --addr 127.0.0.1:0 --dashboard --no-open` (write addr to a file).
2. Read the chosen addr; `http.Get(addr+"/healthz")` → 200.
3. `http.Get(addr+"/")` → 200, body contains `<div id="app">` (dashboard mounted).
4. Send SIGTERM; assert process exits within 5s, exit code 0, stderr has no `panic` or `leak`.

**Test 2 — `e2e/init_apply_list_test.go`**

`TestInitApply_AgentList`:
1. `cd t.TempDir()`; `kave init --org acme --project demo` → writes a `kave.yaml`.
2. Edit fixture: append 2 agents and 1 policy via a textual patch helper (or ship a fixed `kave.yaml.golden` and copy it).
3. `kave apply kave.yaml` → exit 0, stdout reports `created agents=2 policies=1`.
4. `kave agent list -o json` → JSON array length 2, names match fixture.
5. Re-run apply → idempotent, stdout reports `unchanged=2 unchanged=1`.

**Test 3 — `e2e/watch_test.go`**

`TestWatch_ReceivesEvent`:
1. Start `kave start` (background).
2. Start `kave watch --json` (background, capture stdout).
3. Trigger a synthetic event by hitting an endpoint that publishes to the bus (e.g., `kave agent create ...` or a direct API call).
4. Assert: within 2s, `watch` stdout contains a line with the agent's ID and event type. Use line-buffered scanner.

## 5.2 Loadgen — `cmd/loadgen/main.go`

**Subcommand:** `loadgen gateway --rps 500 --duration 60s --target http://127.0.0.1:8080 --token agt_...`.

**Implementation:**
- Token bucket at the chosen RPS (use `golang.org/x/time/rate`).
- Worker pool (default `--workers = 4 × NumCPU`).
- Per request: build minimal OpenAI-shaped chat request, POST `/v1/openai/chat/completions`, read full body, record latency.
- Sample resource snapshots every second: `runtime.NumGoroutine()`, `runtime.MemStats.HeapAlloc`, open FD count (Linux: `/proc/self/fd` len; Mac: `lsof` not used; just skip if not Linux).
- At end, print JSON report to stdout and write to `--out FILE`:
  ```json
  {
    "rps_target": 500, "rps_actual": 499.4, "duration_s": 60,
    "latency_ms": {"p50": 12, "p95": 38, "p99": 71, "max": 134},
    "errors": {"total": 0, "by_class": {}},
    "goroutines": {"start": 42, "end": 43, "max": 47, "drift": 1},
    "fd": {"start": 18, "end": 18, "max": 19, "drift": 0},
    "heap_mb": {"start": 14, "end": 22, "max": 28, "delta": 8}
  }
  ```

**Self-test:** `cmd/loadgen/loadgen_test.go` boots a `harness.New(t)` server + mock upstream, runs `loadgen gateway --rps 50 --duration 5s`, asserts:
- p99 < 100ms,
- error rate 0,
- goroutine drift ≤ 50 from start,
- FD drift ≤ 5,
- heap delta < 50MB.

`make loadgen` runs the 5s self-test against the in-process harness; the 60s × 500 RPS run is a CI-nightly job (spec-05.4).

## 5.3 Contract goldens

Plan 15 closed the HTTP bridge. The remaining HTTP surface is small and stable:
the LLM gateway proxy, `/api/v1/fx/*`, and `/health`. The control plane's wire
contract is now gRPC, gated by `make buf-breaking` against `origin/main` —
that's the canonical contract-review tool, no new infrastructure needed.

This spec adds **two** small contract suites:

### 5.3a HTTP goldens (the remaining surface only)

**File:** `server/internal/contract/contract_test.go`.
Helper: `golden.Equal(t, name, gotBytes)` — handles `-update`, prints unified diff on mismatch.

Goldens to produce, under `server/internal/contract/testdata/`:
- `health.json` — `GET /health`
- `fx_currencies.json` — `GET /api/v1/fx/currencies`
- `fx_rates.json` — `GET /api/v1/fx/rates`
- `fx_convert.json` — `GET /api/v1/fx/convert?from=USD&to=EUR&amount=100`
- `fx_refresh.json` — `POST /api/v1/fx/refresh`
- `gateway_error_unauthorized.json` — `POST /v1/openai/chat/completions` with no auth → 401 body shape
- `gateway_error_provider_not_supported.json` — `POST /frameworks/unknown/whatever` → 400 body shape

Gateway happy-path responses are *upstream-shaped* (they pass through), so
they don't have a stable Kave-owned contract — covered by integration tests
in spec 02 instead.

### 5.3b gRPC wire goldens (control plane)

**File:** `server/port/grpc/contract_test.go`.

For each canonical RPC happy path (org list, agent list, agent get, policy
list, credential list, audit query, trace get, span query, login):
1. Call the RPC via the harness's `bufconn` client with seeded data.
2. Marshal the response with `protojson.MarshalOptions{Multiline: true, Indent: "  ", UseProtoNames: true}.Marshal`.
3. Compare to `server/port/grpc/testdata/contracts/<service>_<rpc>.json`.

The protojson form is the wire contract for any future grpc-gateway transcoding
(post-v1) and gives reviewers a readable diff. `make buf-breaking` covers the
schema; goldens cover the populated-shape (which fields are set on a typical
response, e.g., empty maps vs absent fields).

**Endpoints to audit (one source of truth):** `grep -rn "RegisterServer\|grpc.RegisterService" server/` to enumerate. Add one golden per RPC's representative happy path; skip pure mutations whose response is just a status echo (`Delete`, `Touch`).

## 5.4 CI — `.github/workflows/test.yml`

Two workflows:

```yaml
# .github/workflows/test.yml — on every PR
jobs:
  unit:
    runs-on: ubuntu-latest
    steps:
      - checkout, setup-go (1.23+), cache modules
      - go work sync && make test-unit
      - upload coverage as artifact
  integration:
    runs-on: ubuntu-latest
    services:
      docker: { image: docker:dind }   # or use docker-in-docker action
    env: { INTEGRATION: "1" }
    steps:
      - checkout, setup-go, cache
      - make test-integration
  contracts:
    steps:
      - checkout, setup-go
      - make contracts            # runs the golden tests, fails on diff
  proto-breaking:
    steps:
      - make buf-breaking         # against origin/main
```

```yaml
# .github/workflows/nightly.yml — schedule cron daily 03:00 UTC
jobs:
  bench:
    steps:
      - make bench
      - make bench-compare        # benchstat vs benchmarks/BASELINE.txt
      - fail if any row regresses >15%
      - commit benchmarks/history/<date>.json on green
  e2e:
    steps:
      - make build && make test-e2e
  loadgen:
    steps:
      - make build
      - ./bin/kave start --addr 127.0.0.1:8080 &
      - sleep 2
      - ./bin/loadgen gateway --rps 500 --duration 60s --out report.json
      - python3 ci/check_loadgen.py report.json    # asserts the four invariants
      - upload report.json as artifact, archive to benchmarks/history/
```

`ci/check_loadgen.py` is a 30-line script that reads the JSON and exits non-zero if `latency_ms.p99 > 100`, `errors.total > 0`, `goroutines.drift > 50`, or `fd.drift > 5`.

## 5.5 Makefile additions

```make
test-unit:
	go test -race -count=1 ./...

test-integration:
	INTEGRATION=1 go test -race -count=1 -tags=integration -timeout 5m ./...

test-e2e: build
	go test -tags=e2e -timeout 10m ./e2e/...

bench:
	mkdir -p benchmarks
	go test -bench=. -benchmem -benchtime=2s -count=5 ./... | tee benchmarks/latest.txt

bench-compare:
	benchstat benchmarks/BASELINE.txt benchmarks/latest.txt

loadgen: build
	./bin/loadgen gateway --rps 50 --duration 5s --target http://127.0.0.1:8080

contracts:
	go test -run Contract ./server/internal/contract/... ./server/port/grpc/...

contracts-update:
	go test -run Contract -update ./server/internal/contract/... ./server/port/grpc/...
```

## What an implementing agent should produce

- The three `e2e/*_test.go` files + `e2e/internal/run.go` helper.
- `cmd/loadgen/main.go` + its self-test.
- `server/internal/contract/contract_test.go` + initial set of `testdata/contracts/*.json` (run with `-update` to populate).
- The two CI workflow YAMLs.
- `ci/check_loadgen.py` (Python — keep stdlib-only).
- Makefile targets above.
- Document the loadgen invariants in `cmd/loadgen/README.md` (one short page).
