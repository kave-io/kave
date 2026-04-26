# Plan 15 — HTTP Bridge Reduction + gRPC Migration

**Goal:** delete the HTTP bridge (~3k LOC across 10 files) and complete the gRPC surface that replaces it. Keep HTTP only for the LLM gateway (`server/internal/gateway/`) and `/health`. Wire the 29 remaining CLI stubs and the daemon lifecycle commands against the new gRPC services.

## Status as of 2026-04-26

A previous pass scaffolded part of this work and stopped short:

- `proto/kave/runtime/v1/runtime.proto` was extended with `WatchEvents`, `WatchLogs`, `TailTraces`, `StreamSpans`, `ExportTrace`, `IngestTraces`. Generated code is in `proto/gen/...`.
- `server/app/control/auth_service.go` exists with **real implementations** for Register, Login, Logout, Whoami, ChangePassword, ListSessions, RevokeSession. API-token and agent-token methods inside it are still `Unimplemented`.
- `server/app/control/rbac_service.go` and `daemon_service.go` are skeletons — every method returns `Unimplemented`.
- `server/app/runtime/runtime.go` has the streaming-method skeletons added but they all return `Unimplemented`.
- `server/app/control/control.go` has `RegisterWithChildren` but it is **not called** from `server/main.go` — the new services aren't wired in yet.
- HTTP bridge is **fully present** in `server/internal/httpbridge/`. Not a single route deleted.

So the plan is: finish the implementations, wire them, delete the HTTP bridge, wire the CLI.

## Read first

- `server/app/control/auth_service.go` — pattern to imitate for the remaining services.
- `server/internal/httpbridge/auth_routes.go` — source of truth for what the auth/rbac/sessions/tokens routes currently do; copy the bodies into the gRPC handlers and drop the JSON marshaling.
- `server/internal/httpbridge/routes.go` — same for control surface; most routes already have a gRPC equivalent in `ControlPlaneService`, so the route is just deletable.
- `server/internal/httpbridge/streams.go` — SSE handlers for trace tail, events tail, logs tail, spans stream. Reuse the bus-subscription pattern in the new streaming gRPC methods.
- `server/internal/httpbridge/daemon_routes.go` — what `DaemonService` must cover.
- `server/internal/daemon/daemon.go` — existing daemon `State` (PID, config, store, fx, cost, bus). The lifecycle commands need to spawn/attach to a process that wraps this.
- `cli/internal/commands/agent/get_handler.go` — gRPC handler template for the CLI.
- `cli/internal/commands/lifecycle/*.go` — placeholder handlers; `start_handler.go` and `stop_handler.go` are the most involved.
- `cli/internal/config/` — config-file writer for `ctx use` and `config set`.

## Work breakdown (commit per phase)

### Phase A — Finish RBAC + Daemon gRPC services

`server/app/control/rbac_service.go`:
- `CreateRole`, `ListRoles`, `GetRole`, `DeleteRole` — port from `server/internal/httpbridge/auth_routes.go` createRoleAuth/listRolesAuth/deleteRoleAuth.
- `CreateBinding`, `ListBindings`, `DeleteBinding` — same.
- `TestPermission` — same; calls casbin `Enforce`.

`server/app/control/daemon_service.go`:
- `Status`, `Doctor` — port from `daemon_routes.go` statusRoute/doctorRoute, talking to `*daemon.State`.
- `ConfigView`, `ConfigDiff`, `ConfigPaths`, `ConfigReload` — port the config handlers.
- `Diff`, `Apply` — port apply/diff.
- `AdminStore` — port adminStoreRoute (read-only state dump).

Constructors take `(*daemon.State, store.AppStore, *casbin.Casbin)` as needed. Add tests in the same package using bufconn (see `cli/internal/testutil/grpcharness.go` for client side; for server side just call the handler directly).

Commit: `feat(server): RBAC + Daemon gRPC services`

### Phase B — Streaming runtime methods

`server/app/runtime/runtime.go`:
- `WatchEvents(req, stream)`: subscribe to `eventBus`, forward filtered events as `EventRecord` until ctx cancels.
- `WatchLogs(req, stream)`: subscribe to logsink topic.
- `TailTraces(req, stream)`: poll/watch span store for new trace IDs in env scope.
- `StreamSpans(req, stream)`: subscribe to span events.
- `ExportTrace`: gather all spans + actions for a trace id, return one snapshot.
- `IngestTraces(stream)`: client-streaming handler; persist incoming `SpanRow` messages.

Match the SSE bodies in `server/internal/httpbridge/streams.go` and `trace_routes.go` exactly. Tests use bufconn streaming clients.

Commit: `feat(server): runtime streaming gRPC methods`

### Phase C — Wire services in main + auth interceptor exemptions

In `server/main.go`:
- Construct `auth_service`, `rbac_service`, `daemon_service`.
- Use `controlServer.RegisterWithChildren(authSvc, rbacSvc, daemonSvc)` (the helper that already exists) and pass the result into `portgrpc.New(...)`.

In `server/port/grpc/auth_interceptor.go`:
- Whitelist `AuthService.Register` and `AuthService.Login` from authentication — these MUST be reachable without a token. Match by full method name (`/kave.control.v1.AuthService/Login`).

Commit: `feat(server): wire auth/rbac/daemon services + interceptor exemptions`

### Phase D — Delete the HTTP bridge

Once Phases A–C ship and tests pass:
- Delete `server/internal/httpbridge/auth_routes.go`, `routes.go`, `streams.go`, `trace_routes.go`, `daemon_routes.go`, `bridge.go`, `jsonmodel.go`, `bridge_test.go`.
- Decide on `auth_middleware.go`: if the LLM gateway needs to authenticate inbound LLM calls, keep just the bearer-PASETO branch. If not, delete the whole file.
- In `server/main.go`, remove `httpbridge.Register(...)`, `RegisterStreams`, `RegisterTraceRoutes`. Keep `gateway.RegisterRoutes(mux)`, `fx.RegisterRoutes(mux, fxService)`, and `/health`.
- Delete `cli/internal/commands/admin/store/` if it was only used to call the deleted bridge route, or rewire it to `DaemonService.AdminStore`.

Commit: `refactor: delete HTTP bridge (gRPC-only control plane)`

### Phase E — Fill the 29 remaining CLI stubs

For each, follow the template in `cli/internal/commands/agent/get_handler.go`:

- **lifecycle**: `start_handler.go`, `stop_handler.go`, `watch_handler.go`, `logs_handler.go`. `start` spawns the server detached, writes `~/.kave/daemon.pid`. `stop` reads PID and SIGTERMs. `watch`/`logs` open the new streaming RPCs and accumulate until limit/timeout.
- **rbac** + **rbac/role**: list, grant, revoke, role create/list/get/delete → `RBACService`.
- **events**: list, tail → `RuntimeService.WatchEvents` (one-shot snapshot vs. streaming).
- **trace**: tail, graph, export → `TailTraces`, `ExportTrace`.
- **span**: tail, export → `StreamSpans`.
- **connector**: list, get, enable, disable, test — these still have NO RPC. Either add `ConnectorService` to proto or leave as `unavailable` + document.
- **price**: refresh, import, export, fx/* — same; no RPC. Same call.
- **apply**, **credential test**, **policy test**: no RPC; leave unavailable + document why.
- **ctx use**: rewrite the active-server pointer in the config file using `cli/internal/config` writers.
- **config set**: same; write `key = value` into the config.

Tests: bufconn coverage for each new handler, plus a config-file-mutation test for `ctx use` and `config set`.

Commit: `feat(cli): fill lifecycle + rbac + events + trace/span streaming handlers`

## Acceptance

- `find server/internal/httpbridge -type f | wc -l` is small (≤2 files: maybe `auth_middleware.go` if kept).
- `grep -rln "command.unavailable" cli/ | grep -v _test | wc -l` is ≤ the genuinely-unbacked count (connector/price-mutation/apply/test handlers).
- `kave start; kave watch` works against a freshly-spawned daemon.
- `kave ctx use foo` actually flips the active server in `~/.kave/config`.
- `go build ./...` and `go test ./...` clean across all modules.

## Out of scope

- `ConnectorService` and `PriceCatalogService` proto additions — track separately; only add if you have time after the rest.
- Dashboard frontend (plan 12).
- Windows daemon supervision (Linux + macOS only for v1).

## Size estimate

- Phase A: ~400 LOC + 200 LOC tests.
- Phase B: ~350 LOC + 150 LOC tests.
- Phase C: ~80 LOC main wiring + 40 LOC interceptor whitelist + 60 LOC tests.
- Phase D: net **−2,500 LOC** (deletion).
- Phase E: ~600 LOC across 29 handlers + 400 LOC tests.

Net delta after the whole plan: roughly flat (deletions ≈ additions), but the surface is simpler and gRPC-only. One haiku session per phase, with verification between.
