# Plan 10 — CLI Fillout: Close the ~98 NotImplemented Handlers

**Goal:** finish the CLI so every `kave <noun> <verb>` command actually works against the HTTP bridge. Today `grep -rln "output.NotImplemented" cli/` returns ~98 stub handlers. The server surface (HTTP bridge + gRPC) now exists; the CLI just needs to call it. Also rebuild `kave watch` on top of existing SSE streams since `/api/v1/watch` was removed.

## Read first

- `cli/internal/commands/` — one subdirectory per noun (agent, policy, credential, budget, connector, auth, rbac, trace, span, events, config, ctx, apply, price, token). Each `<verb>_handler.go` is an `output.NotImplemented(...)` stub today.
- `cli/internal/runtime/runtime.go` — shared HTTP client, base URL resolution from `~/.kave/config` + flags.
- `server/internal/httpbridge/routes.go` — canonical list of server endpoints and their shapes. Most CLI handlers are thin wrappers around these.
- `cli/internal/output/output.go` — `Render(cmd, out, verb)` is the shared table/json/yaml printer. Every handler returns a typed output struct; `Render` formats it.
- `cli/internal/flags/page.go` — `--limit`, `--cursor` wiring. Reuse everywhere.
- `cli/cmd/root.go` — global flags + see_also metadata.

## Design

### Handler anatomy (template)

Every handler is three pieces: Input struct, Output struct, `Run<Verb>(ctx, in)` that calls the HTTP client. Current stubs already have the structs — just replace the body.

```go
// cli/internal/commands/agent/list_handler.go
func RunList(ctx context.Context, in ListInput) (*ListOutput, error) {
    c := runtime.Client()
    resp, err := c.Get(ctx, "/api/v1/agents", runtime.Query{
        "limit": in.Limit, "cursor": in.Cursor, "project_id": in.ProjectID,
    })
    if err != nil { return nil, err }
    var env contract.PageEnvelope[contract.Agent]
    if err := resp.JSON(&env); err != nil { return nil, err }
    return &ListOutput{Agents: env.Data.Items, NextCursor: env.Data.NextCursor}, nil
}
```

Conventions:
- Always pass through `--limit` / `--cursor` when the server supports it.
- `--project`, `--env`, `--agent` resolve through `ctx` (`kave ctx use <project>`) unless explicitly overridden.
- Stream verbs (`tail`, `watch`) use `runtime.Stream(ctx, path, query)` which wraps the JSONL SSE decoder — contracts already defined in `contract.stream`.

### Batching by noun

Group implementations by noun, ship as a series of commits inside the single plan session:

1. **agent** — list / get / create / update / delete / restore / export / token {issue,list,revoke}
2. **policy** — list / get / create / update / delete / export / validate / test
3. **credential** — list / get / create / update / delete / rotate / test
4. **budget** — show / set / list-entries / reset
5. **connector** — list / enable / disable / test / status
6. **auth** — login / logout / whoami / token {list, revoke}
7. **rbac** — role {list,create,delete} / binding {list,create,delete}
8. **price** — book / import / export / refresh / get
9. **trace** — list / get / tail / graph / export
10. **span** — get / tail / export
11. **events** — list / tail
12. **config** — view / diff / reload / get / set / path
13. **apply** / **diff** — apply engine (expects `/api/v1/apply` on the server — if missing, stub apply with a TODO pointing at a follow-up plan; do NOT leave silent NotImplementeds)
14. **status** / **doctor** / **logs** — daemon lifecycle, already partially wired

### kave watch — compose, don't re-endpoint

`kave watch` replaces the dedicated endpoint. It tails multiple streams concurrently and merges into one output:

```go
func RunWatch(ctx, in) error {
    ch := make(chan output.Row, 64)
    go stream("/api/v1/spans/stream", ch)
    go stream("/api/v1/events/stream", ch)    // from plan 03
    go stream("/api/v1/logs/stream", ch)      // from plan 04
    for row := range ch { render(row) }
}
```

Filters (`--agent`, `--project`, `--env`, `--level`) pass through to each stream's query params.

### JSON/Table/YAML parity

Every handler's Output struct implements `output.TableRenderable` (returns header + rows) so `--output json|yaml|table` works without per-handler code. Most are trivial — existing structs already have the fields.

### Progress/interactivity

- `kave apply` renders a plan (diff) and prompts `[y/N]` unless `--yes`.
- `kave credential create --interactive` prompts for secret via stdin without echo (`golang.org/x/term`). Never log secret values.
- `kave auth login` prompts for email + password, stores the returned PASETO in OS keyring (reuse `core/pkg/keyring`).

### Error surface

Every non-2xx server response surfaces as `contract.ErrorEnvelope{Code, Message, Hint}`. `cli/internal/errors` maps `Code` → exit code:
- `auth.*` → 77 (auth required)
- `gateway.policy_blocked` → 75
- `gateway.budget_exceeded` → 74
- validation errors → 64
- anything else → 1

## Files

Touch ~100 files across `cli/internal/commands/*/` (`*_handler.go`). Shared infra changes:
- `cli/internal/runtime/runtime.go` — add `Stream(ctx, path, query)`, `Post`, `Put`, `Delete` helpers if missing.
- `cli/internal/contract/` — mirror or re-export server contract types.
- `cli/internal/ctx/ctx.go` — resolve `project/env/agent` defaults from `~/.kave/context.yaml`.

Do NOT modify server code in this plan — if a handler would need a new server endpoint, stop and file it as a follow-up; don't paper over with NotImplemented.

## Acceptance

- `grep -rln "output.NotImplemented" cli/` returns **0**.
- Every noun has a passing integration test in `cli/internal/commands/<noun>/handler_test.go` that boots the server in-process (httptest) and exercises list+get+create+delete.
- `kave watch` merges spans/events/logs in real time; clean shutdown on Ctrl-C.
- `kave apply kave.yaml --dry-run` prints a deterministic diff.
- `kave auth login` stores session via keyring; `kave auth whoami` round-trips.
- Outputs pass JSON-schema validation against the contract docs (spot-checked via `cli/testdata/*.schema.json`).
- `go build ./... && go test ./...` clean.

## Out of scope
- `kave ui` local dashboard launch beyond `open http://localhost:PORT`.
- Shell completion rework (existing cobra completion is fine).
- Plugin architecture for external commands (post-v1).

## Size estimate
~2500 LOC across ~100 files, but most handlers are 15–25 lines. Heavy repetition → one haiku session if it works noun-by-noun and reuses the template aggressively.
