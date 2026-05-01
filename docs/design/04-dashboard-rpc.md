# Dashboard RPC Contract

Status: accepted for v1 implementation
Scope: embedded Vue/Vite daemon dashboard, CLI transport, future Kave Cloud dashboard

## Decision

Kave dashboards should use the protobuf API contract directly through ConnectRPC-style browser clients.

The dashboard must not grow a handwritten REST API layer such as:

```txt
/api/v1/agents
/api/v1/runs
/api/v1/spans
/api/v1/cost/summary
```

Those endpoints duplicate the existing control/runtime contract and fight Kave's architecture goal: one canonical contract for CLI, daemon, dashboard, and cloud.

## Target Shape

```txt
proto/*.proto
  ↓ buf generate

proto/gen
  Go protobuf types
  Go RPC service stubs

dashboard/src/gen
  TypeScript protobuf/client types

server/app/*
  business services and mappers

server/port/grpc
  existing CLI/internal gRPC transport

server/port/connect
  browser-compatible HTTP RPC transport

dashboard/src/lib/rpc
  generated-client transport, auth, timeout, error mapping

dashboard/src/views/*
  call typed RPC clients, not fetch("/api/...")
```

The browser transport should use Connect-Web / Connect-ES style clients over normal HTTP `fetch`.

The Go daemon should expose browser-compatible RPC routes from `server/port/connect`, backed by the same `server/app/control`, `server/app/runtime`, `server/app/audit`, and `server/app/fx` services already used by the existing gRPC port.

## Current Architecture Alignment

Kave already has the right separation:

```txt
core/
  domain models, runtime contracts, mappers, stores, pipeline
  no browser, no HTTP UI concerns

server/app/
  application services:
    control/
    runtime/
    audit/
    fx/

server/port/grpc/
  transport edge for current gRPC clients

server/ui/
  embedded dashboard dist via go:embed

dashboard/
  Vue + Vite static app

cli/internal/runtime/
  CLI transport/runtime client code
```

The new dashboard RPC path should add transport code at the edge only.

Do not put RPC/client/browser logic into `core`.

Do not put dashboard-specific DTOs into `server/app`.

Do not add REST-shaped API handlers unless there is a separate public REST product decision.

## Repository Changes

### 1. Add Connect/browser RPC port

Create:

```txt
server/port/connect/
  server.go
  auth_interceptor.go
  cors.go
  errors.go
```

This package should:

- mount generated RPC handlers on `net/http`
- reuse `server/app/*` services
- apply auth/context interceptors
- map domain errors to RPC error codes
- support server-streaming methods for live dashboard views

`server/main.go` should mount:

```txt
/health
/gateway/provider routes
/connect or generated RPC service routes
/static embedded dashboard
```

Prefer unprefixed generated RPC routes for local daemon compatibility unless routing gets messy. If a prefix is needed, use `/rpc`.

### 2. Keep `server/port/grpc` during migration

The existing gRPC transport is already used by CLI tests and commands.

Do not delete it during the dashboard migration.

Short-term:

```txt
CLI          -> server/port/grpc
Dashboard    -> server/port/connect
Both         -> server/app/*
```

Long-term, if Connect-Go fully covers CLI needs, `server/port/grpc` can be folded into a unified `server/port/rpc`.

### 3. Generate TypeScript clients for dashboard

Update `buf.gen.yaml` to generate browser TypeScript code in addition to Go code.

Current:

```yaml
plugins:
  - local: protoc-gen-go
    out: proto/gen
    opt: paths=source_relative
  - local: protoc-gen-go-grpc
    out: proto/gen
    opt: paths=source_relative
```

Target direction:

```yaml
plugins:
  - local: protoc-gen-go
    out: proto/gen
    opt: paths=source_relative

  - local: protoc-gen-go-grpc
    out: proto/gen
    opt: paths=source_relative

  - local: protoc-gen-connect-go
    out: proto/gen
    opt: paths=source_relative

  - local: protoc-gen-es
    out: dashboard/src/gen
    opt: target=ts
```

Later, when Kave Cloud lands in the same workspace, move generated TS to a shared package:

```txt
packages/kave-proto-web/src/gen
packages/kave-rpc-client/src
dashboard/
cloud-dashboard/
```

For now, generating into `dashboard/src/gen` is simpler and matches the current repo.

### 4. Replace dashboard REST helpers

Current dashboard files to migrate:

```txt
dashboard/src/lib/fetch.ts
dashboard/src/lib/queries.ts
dashboard/src/types/api.ts
dashboard/src/composables/useSpanStream.ts
```

Target:

```txt
dashboard/src/lib/rpc/transport.ts
dashboard/src/lib/rpc/clients.ts
dashboard/src/lib/rpc/errors.ts
dashboard/src/composables/useAgents.ts
dashboard/src/composables/useRuns.ts
dashboard/src/composables/useSpans.ts
dashboard/src/composables/useCost.ts
dashboard/src/composables/useRunStream.ts
dashboard/src/composables/useSpanStream.ts
```

Rules:

- no handwritten API response types when generated protobuf types exist
- no `fetch("/api/v1/...")`
- all RPC calls go through `dashboard/src/lib/rpc`
- composables own loading/error/retry state
- views only consume composables

## RPC Service Boundaries

Map dashboard screens to existing service areas:

```txt
AgentsView.vue
  control service
  ListAgents
  GetAgent
  CreateAgent
  UpdateAgent
  DeleteAgent

PoliciesView.vue
  control service
  ListPolicies
  GetPolicy
  CreatePolicy
  UpdatePolicy
  DeletePolicy
  ValidatePolicy

RunsView.vue
  runtime service
  ListRuns
  GetRun
  WatchRuns

TracesView.vue
  runtime/trace service
  ListTraces
  GetTrace
  ListSpans
  WatchSpans

IndexView.vue
  runtime + cost/fx service
  GetCostSummary
  ListRecentRuns
  ListRecentAlerts

SettingsView.vue
  control/auth/fx service
  WhoAmI
  ListContexts
  ListCredentials
  ListConnectors
  GetFXRates
```

Prefer read-model RPCs that are useful to both CLI and dashboard.

Avoid creating dashboard-only RPCs unless they are true aggregate read models, for example:

```txt
GetDashboardOverview
GetCostSummary
GetTraceGraph
```

## Streaming

Browser dashboards should use server-streaming RPCs.

Good:

```proto
rpc WatchRuns(WatchRunsRequest) returns (stream RunEvent);
rpc WatchSpans(WatchSpansRequest) returns (stream SpanEvent);
rpc WatchEvents(WatchEventsRequest) returns (stream EventRecord);
```

Avoid browser bidirectional streaming for v1.

The current dashboard stream code:

```txt
dashboard/src/composables/useSpanStream.ts
```

should move from ad-hoc HTTP/SSE-style behavior to generated streaming RPC clients.

CLI commands such as:

```txt
cli/internal/commands/lifecycle/watch_handler.go
cli/internal/commands/span/tail_handler.go
cli/internal/commands/trace/tail_handler.go
cli/internal/commands/events/tail_handler.go
```

should eventually use the same watch/tail RPC concepts.

## Embedded Dashboard

The embedded dashboard remains Vue + Vite.

Keep:

```txt
dashboard/
server/ui/embed.go
server/ui/dist
```

Build flow remains:

```txt
make dashboard-build
make build
```

Production should serve dashboard and RPC from the same daemon origin:

```txt
http://127.0.0.1:18080/
http://127.0.0.1:18080/<rpc-service>/<method>
```

This avoids production CORS.

In dev, Vite may proxy RPC calls to the Go server.

## Kave Cloud

Kave Cloud should use the same generated TypeScript RPC client.

Do not create a separate cloud-only API contract for core Kave resources.

Cloud may add extra services for:

```txt
orgs
billing
subscriptions
teams
invitations
cloud auth/session
hosted workspaces
```

But core resources still come from the same proto family:

```txt
agents
policies
credentials
runs
spans
traces
budgets
cost
audit
fx
```

Nuxt is acceptable for Kave Cloud because it needs auth, billing, account flows, SSR, and cloud deployment behavior.

The embedded daemon dashboard should stay Vite because it is static, small, and easy to embed.

## Architecture Linter Alignment

The existing HTTP allowlist rule protects this decision.

`B10-http-allowlist` should continue blocking accidental REST route growth.

Allowed HTTP surfaces should remain limited to:

```txt
/health
provider gateway routes
embedded UI assets
generated RPC routes
```

Do not add:

```txt
/api/v1/*
```

unless there is an explicit design document for a public REST API.

If the linter is expanded from `HandleFunc` to all `Handle` calls, generated RPC service routes should be allowlisted as RPC transport routes, not REST API routes.

## Error Handling

RPC error codes should become the dashboard's standard error model.

Recommended mapping:

```txt
Unauthenticated      -> login/session expired
PermissionDenied    -> access denied
InvalidArgument     -> form validation error
NotFound            -> empty/deleted resource
FailedPrecondition  -> policy/budget/trust-mode block
ResourceExhausted   -> budget/rate limit
DeadlineExceeded    -> timeout with retry
Unavailable         -> daemon offline
Internal            -> generic failure with request id
```

Domain errors should be mapped once in the transport layer.

Views should not parse raw backend error strings.

## Auth and Context

Every dashboard RPC should carry:

```txt
Authorization, if enabled
workspace/context id
project id, when applicable
environment id, when applicable
request id
timeout/deadline
```

Server-side transport should resolve identity through existing auth context code:

```txt
server/internal/authctx
server/ops/auth
server/internal/infra/paseto
server/internal/infra/casbin
```

Policy code should continue consuming identity from context.

This preserves the existing auth/policy separation enforced by the architecture linter.

## Money and Time

Generated browser types must respect existing Kave rules:

```txt
No float money.
No time.Time in core models.
Use integer/string-safe money and unix-ms timestamps.
```

Dashboard formatting belongs in:

```txt
dashboard/src/lib/money.ts
dashboard/src/lib/time.ts
```

Backend canonical values remain in proto/core model types.

Special care:

```txt
TOMAN support must remain first-class.
64-bit values should be browser-safe.
Large counters and nanos should be strings if needed.
```

## Migration Plan

### Phase 1: Add RPC transport

- add `server/port/connect`
- generate Connect-Go handlers
- expose read-only RPCs needed by dashboard
- keep existing gRPC port and CLI behavior unchanged

### Phase 2: Generate dashboard TS types

- add `protoc-gen-es`
- generate into `dashboard/src/gen`
- add `dashboard/src/lib/rpc/transport.ts`
- add typed client exports

### Phase 3: Replace dashboard fetch layer

Migrate in this order:

```txt
IndexView.vue
RunsView.vue
TracesView.vue
AgentsView.vue
PoliciesView.vue
SettingsView.vue
```

Delete or shrink:

```txt
dashboard/src/lib/fetch.ts
dashboard/src/types/api.ts
```

### Phase 4: Replace stream code

- migrate `useSpanStream.ts` to server-streaming RPC
- add `WatchRuns`
- align CLI tail/watch commands with the same event model

### Phase 5: Remove REST assumptions

- remove `/api/v1/*` dashboard assumptions
- keep B10 strict
- document generated RPC routes as the only dashboard backend API

## Final Rule

Kave should have one product contract:

```txt
protobuf first
generated clients
transport edges only at server/port/*
application logic only in server/app/*
domain logic only in core/*
dashboard consumes typed RPC, not REST
```

This keeps the local daemon, CLI, embedded dashboard, and Kave Cloud aligned without duplicating API surfaces.
