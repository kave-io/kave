# Plan 02 — HTTP JSON Bridge (thin, gRPC-first)

**Goal:** expose the full CLI surface over HTTP *without* hand-writing one handler per endpoint. We already own gRPC; HTTP should be a transcoder on top, with explicit handlers only for things gRPC-transcoding can't do well (SSE/streaming, file uploads, passthrough proxy).

## Design — read first
We are **not** adopting full `grpc-gateway` (it drags proto annotations, buf plugins, and per-field option churn). Instead:

- Keep current hand-written JSON envelope (`server/internal/contract`).
- Add one generic reflection-based transcoder in `server/internal/httpbridge/`:
  1. At startup, walk registered gRPC services (or build a manual table of `{method, path, svc.Method}` pointers — *manual table is simpler and avoids reflection on unary methods*).
  2. For each unary RPC, register a `mux.HandleFunc(method, path, bridgeFunc(svcMethod))`.
  3. `bridgeFunc`: decode JSON → proto via `protojson.Unmarshal`, call the in-process gRPC handler directly (no network hop), encode response via `protojson.Marshal`, wrap in `SuccessEnvelope`.
- Streaming RPCs and proxy routes stay as explicit handlers.
- Keep the existing hand-written REST handlers (`server/api/*`) during migration. Delete them in a follow-up once the bridge covers their URLs.

This gives us full CLI coverage for ~200 LOC of bridge infra + a route table.

## Scope (single session)

### 1. Build the bridge skeleton
`server/internal/httpbridge/bridge.go`:
```go
type Route struct {
    Method  string             // "POST"
    Path    string             // "/api/v1/agents"
    Invoke  InvokeFn           // takes *http.Request, returns proto.Message + error
    Kind    string             // "Agent"
}
type InvokeFn func(ctx context.Context, body []byte, query url.Values, path map[string]string) (proto.Message, error)

func Register(mux *http.ServeMux, routes []Route)
```

- `Register` wraps each `Invoke` with: JSON envelope encoding (`contract.WriteSuccess/WriteError`), error mapping (`status.FromError` → HTTP code + contract code, e.g. `codes.NotFound` → `404 + "*.not_found"`), content-type handling.
- Single shared helper `protoToJSON(msg proto.Message) []byte` using `protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}`.

### 2. Route table
`server/internal/httpbridge/routes.go`: builds `[]Route` from the control+runtime servers. Each entry looks like:
```go
{
    Method: "GET", Path: "/api/v1/agents/{id}", Kind: "Agent",
    Invoke: func(ctx, _, _, pv) (proto.Message, error) {
        return ctrl.GetAgent(ctx, &controlv1.GetAgentRequest{Id: pv["id"]})
    },
},
```
Cover the full set required by `docs/01-cli-spec.md`: agents, policies, credentials, runs, spans (list/get; tail is streaming — skip here), cost, pricing, orgs/projects/envs, budgets, tokens.

### 3. List-cursor plumbing
For `List*` endpoints, parse `limit` and `cursor` query params into the proto request. Response mapping: extract `NextCursor` from the proto response (use `protoreflect` to read the field by name, or switch on message type — table-driven is fine since it's known at registration time), and pass it to `pagedResponseJSON`.

Preferred approach: add a `PageResp` interface `{ GetNextCursor() string }` to every List* response via proto; the generated getter makes this trivial. Route entry carries a `ListKind bool` flag and the bridge handles paging uniformly.

### 4. Error mapping
Central table in bridge:
- `codes.InvalidArgument` → 400 / `request.invalid`
- `codes.NotFound` → 404 / `<kind>.not_found`
- `codes.AlreadyExists` → 409 / `<kind>.already_exists`
- `codes.PermissionDenied` → 403 / `auth.forbidden`
- `codes.Unauthenticated` → 401 / `auth.unauthenticated`
- `codes.Unimplemented` → 501 / `server.unimplemented`
- default → 500 / `server.internal`

### 5. Wire into main
In `server/main.go`, after registering existing routes:
```go
httpbridge.Register(mux, httpbridge.BuildRoutes(ctrlServer, runtimeServer))
```
Keep existing `api.New(...)` handlers registered — but they will be masked by identical paths in the bridge. Audit overlap: if paths collide, prefer bridge; delete the legacy file in a follow-up cleanup commit at the end of this session.

### 6. Streaming / non-transcoded endpoints
Leave these explicit:
- `GET /api/v1/spans/stream` (SSE) — already exists; move it into `server/internal/httpbridge/streams.go` for cohesion.
- `/frameworks/claude-code/*` — gateway; untouched.
- `GET /api/v1/trace/tail`, `/api/v1/events/tail`, `/api/v1/logs/tail` — **stub these to return 501** in this plan. Plan 03 implements them.

### 7. CLI coverage endpoints that HAVE no gRPC yet
For `status`, `doctor`, `config view/reload`, `admin store`, `apply`, `diff`, `watch` — add placeholder routes returning 501 with `server.unimplemented`. Plan 04 / 05 fills them in. Log a WARN at startup listing unimplemented CLI paths.

## Files to touch / create
- `server/internal/httpbridge/bridge.go` (new)
- `server/internal/httpbridge/routes.go` (new)
- `server/internal/httpbridge/streams.go` (move SSE here)
- `server/main.go` (wire-up)
- `server/api/*.go` — leave alone this session (to minimize blast radius); audit collisions only.

## Acceptance
- `go build ./... && go test ./...` clean across modules.
- `curl localhost:<port>/api/v1/agents?env_id=default` returns the same envelope shape as before.
- `curl -X POST localhost:<port>/api/v1/agents -d '{"project_id":"default","env_id":"default","name":"x"}'` creates an agent via the bridge path.
- `List*` with `?limit=1&cursor=<prev_next_cursor>` advances pages.
- Bridge error mapping verified by one table-driven test in `server/internal/httpbridge/bridge_test.go` (inject a fake gRPC handler returning each `codes.*`).
- No duplicate JSON encoder path; everything goes through `contract.WriteSuccess/WriteError`.

## Explicitly out of scope
- Auth (Plan 06).
- Streaming endpoints other than existing SSE (Plan 03).
- Config/doctor/apply endpoints (Plan 04/05).
- Deleting legacy `server/api/*` handlers — leave for a final cleanup commit after Plan 08.

## Size estimate
~400 LOC new + ~100 LOC wiring. One haiku session if the agent builds the route table incrementally (agents first, verify curl, then the rest).
