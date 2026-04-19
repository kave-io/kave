# Plan 04 — Daemon Lifecycle: status, doctor, reload, config-diff

**Goal:** fill the operational surface the CLI needs to manage a running kave daemon. All endpoints are unary JSON on the bridge (no streaming — Plan 03 covers that); they plug into the existing `httpbridge.Route` table.

## Read first

- `server/internal/config` — current single-layer Viper load. We will *not* rewrite the config loader here (Plan 05). We only surface what's already loaded plus a reload trigger.
- `server/main.go` — construction currently happens top-to-bottom in `main()`. We need a long-lived `daemon.State` struct that owns the reloadable bits (config, stores, fx, cost, pipeline) so introspection endpoints can report on them.
- `storeManager`, `fxService`, `costService` — each has a `Health()`-shaped method or is cheap to probe. Use what exists; don't invent new healthcheck APIs.

## Design

### 1. `server/internal/daemon` package
New package holding process-level state:

```go
type State struct {
    PID        int
    StartedAt  time.Time
    Version    string           // from build ldflags; "dev" if unset
    SchemaVer  string           // contract.SchemaVersion

    mu         sync.RWMutex
    cfg        *config.Config
    stores     *storeimpl.Manager
    fx         *fx.Service
    cost       *cost.Service
    bus        *bus.Bus
    reloadCh   chan struct{}
}

func New(...) *State
func (s *State) Snapshot() Status
func (s *State) Reload(ctx context.Context) (ReloadReport, error)
func (s *State) Doctor(ctx context.Context) DoctorReport
```

`main.go` constructs `daemon.State` and passes it to the bridge.

### 2. Status endpoint — `GET /api/v1/status`
Returns (envelope `Status`):
```json
{
  "pid": 12345, "version": "dev", "schema_version": "1.0",
  "uptime_ms": 3600000,
  "started_at": "...", "started_at_ms": ...,
  "grpc_addr": ":50051", "http_addr": ":8080",
  "stores": {"app": "ok", "span_default": "ok"},
  "fx": {"loaded": true, "last_refresh_ms": ..., "stale": false, "age_ms": ...},
  "pricing": {"version": "2026-02-01", "models": 42},
  "connectors": {"claude-code": "ready"},
  "bus": {"subscribers": 3}
}
```
- Store health: call `appStore.Ping(ctx)` (add if missing, trivial `SELECT 1`).
- Connector readiness: for now, static list from registered frameworks — full probes land in Plan 07.
- Bus subscriber count: add `(*Bus).SubscriberCount() int` (read-locked len).

### 3. Doctor endpoint — `GET /api/v1/doctor`
Runs the check list and returns an array of results:
```json
{
  "checks": [
    {"name":"daemon","status":"ok","detail":""},
    {"name":"app_store","status":"ok","detail":""},
    {"name":"span_store_default","status":"ok","detail":""},
    {"name":"fx_fresh","status":"warn","detail":"rates are 26h old"},
    {"name":"pricing_loaded","status":"ok","detail":""},
    {"name":"credentials_resolve","status":"ok","detail":"0 dangling refs"},
    {"name":"config_merge","status":"ok","detail":""}
  ],
  "overall": "warn"
}
```
Check runner is a `[]Check` slice; each `Check{Name, Run(ctx) (Status, Detail)}`. Parallelize with `errgroup` bounded to 4. `overall` = worst of (ok < warn < fail).

### 4. Reload — `POST /api/v1/config/reload`
- Re-reads config from disk (whatever `config.MustReadConfig` does today — no layering yet).
- Validates: if new cfg fails validation, return 409 `config.invalid`, keep old cfg live.
- Applies: swap `State.cfg` under lock. Things that *can* hot-reload now: fx interval, pricing path, log level. Things that *cannot*: listen addresses, DB DSN — return those as `requires_restart` in the response.
- Response body: `ReloadReport{ Applied []string, RequiresRestart []string, Warnings []Warning }`.
- Also trigger on SIGHUP: `signal.Notify(sigCh, syscall.SIGHUP)` goroutine calls `State.Reload`.

### 5. Config endpoints
- `GET /api/v1/config/view` — returns the currently-live merged config as JSON (redact secrets: any key matching `(?i)(secret|token|password|key)` replaced with `"***"`).
- `GET /api/v1/config/diff` — compares on-disk config vs live. Returns JSON patch-ish: `{added, removed, changed}` trees. Use `encoding/json` round-trip + a small map-diff helper; avoid pulling in a diff library.

### 6. Admin store — `GET /api/v1/admin/store`
Read-only introspection per store: rows-per-table counts, DB path/DSN (redacted), sqlite file size if applicable. Single handler; delegate to each store's `Stats(ctx)` method (add if missing — simple `COUNT(*)` per known table).

### 7. Bridge wiring
Add entries to `httpbridge.BuildRoutes` (or a sibling `BuildDaemonRoutes(state *daemon.State)` called from main). All of these are *not* gRPC-backed — they call `state` methods directly, wrapped in the same `Outcome` shape the bridge already uses. This means the bridge keeps one uniform handler path; no special casing.

Remove the `log.Printf("warn: unimplemented HTTP bridge routes: ...")` line in main — these endpoints are now real. `apply`, `diff`, `watch` stay 501 (Plan 05/09).

## Files

Create:
- `server/internal/daemon/daemon.go` — State, Snapshot, Reload, Doctor, helpers.
- `server/internal/daemon/checks.go` — doctor check list.
- `server/internal/daemon/diff.go` — map-diff helper.
- `server/internal/httpbridge/daemon_routes.go` — bridge entries.

Modify:
- `server/main.go` — instantiate `daemon.State`, register routes, install SIGHUP handler.
- `core/bus/bus.go` — add `SubscriberCount() int` (from Plan 03 if already present, reuse).
- `server/internal/store/*` — add `Ping(ctx)` / `Stats(ctx)` where missing (keep minimal).

## Acceptance

- `curl /api/v1/status | jq` shows non-zero uptime and correct addresses after 5s.
- `curl /api/v1/doctor` returns all `ok` on a clean dev boot. Stopping postgres (if in use) flips `app_store` to `fail` within one request.
- `curl -XPOST /api/v1/config/reload` applies a changed `fx.refresh_interval` without restart; changing `server.addr` returns it under `requires_restart`.
- `kill -HUP <pid>` triggers the same reload path (verify via logs).
- `curl /api/v1/config/view` never contains raw secret values (test with a config that has `security.encryption_key`).
- `curl /api/v1/config/diff` returns empty-diff after reload with no file changes.
- `go build ./... && go test ./...` clean.

## Out of scope
- Layered config merge (Plan 05).
- Config-watched reload (Plan 05 — here it's manual/sighup only).
- `apply`/`diff`/`watch` resource endpoints (Plan 05 covers apply engine).
- Connector probes beyond "registered" (Plan 07).

## Size estimate
~600 LOC. One haiku session: state skeleton → status → doctor → reload → config view/diff → admin store → SIGHUP → tests.
