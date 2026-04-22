# Plan 09 — Auth Enforcement: Interceptor Wiring, Bearer-UUID Kill, Credential Resolution

**Goal:** close the last auth gap before v1 is actually *enforced*. Plan 06 built the APIs (users, sessions, tokens, casbin bindings); they currently sit dormant. This plan wires them into the request path so unauthenticated or unauthorized calls are rejected, and finishes credential resolution so upstream LLM calls use real secrets, not placeholders.

## Read first

- `server/main.go:86-91` — current pipeline: `policy → budget → trace`. No auth interceptor.
- `server/internal/authctx/authctx.go` — request-scoped identity container. Populated by HTTP middleware but not consumed in the gateway.
- `server/internal/gateway/gateway.go` — `handleFramework` / `handleRaw`: today they accept any bearer UUID that matches an `AgentToken`. Zero-config (no token) falls through to the default agent. Both shortcuts must remain available behind `cfg.Security.AllowAnonymous` but otherwise be gated.
- `server/internal/infra/casbin/*` — real casbin engine with model/policies. Loaded at startup but nothing calls `Enforce`.
- `server/ops/auth/casbin_engine.go` — a *second* casbin wrapper. One of these two must die (see plan 11).
- `server/ops/auth/credresolve/resolve.go:25` — `vault resolution unimplemented`. Only env+passthrough+encrypted work today.
- `core/pkg/keyring/keyring.go` — scaffold for local OS-keyring-backed secret storage; partially filled by plan 06. Used by `credresolve` for encrypted credentials.
- `server/internal/httpbridge/auth_routes.go` — `/api/v1/auth/login`, `/sessions`, `/tokens` handlers already return PASETO tokens.

## Design

### 1. Auth interceptor in the pipeline

New package `server/ops/auth/interceptor.go`:

```go
type Interceptor struct {
    store       store.AppStore
    casbin      *casbin.Engine  // THE one (see plan 11 decision)
    anonAllowed bool            // cfg.Security.AllowAnonymous
}

func (i *Interceptor) Before(ctx, action) (*runtime.Action, error) {
    id := authctx.From(ctx) // set by HTTP/gRPC middleware
    switch {
    case id.IsAgentToken():
        // already resolved; decorate action with agent/project/env from the token
    case id.IsUser():
        // user-driven action (e.g., CLI apply). Enforce casbin on (user, action).
    case id.IsAnonymous() && i.anonAllowed:
        // event-mode: assign to default agent (existing behavior)
    default:
        return nil, ErrUnauthenticated
    }
    if blocked := i.casbin.Enforce(id.Subject(), action.Connector+"."+action.Method, action.Method); !blocked.Allow {
        return nil, ErrUnauthorized{Reason: blocked.Reason}
    }
    return action, nil
}
```

Register **before** policy in `main.go`: `auth → policy → budget → trace`. Order matters — policy/budget assume the action has a real AgentID/ProjectID/EnvID.

### 2. HTTP identity middleware

`server/internal/httpbridge/auth_middleware.go`:

- Parses `Authorization: Bearer <paseto>` → `authctx.Identity`.
- Parses `Authorization: Bearer kav_<token>` → `authctx.Identity` (agent token path).
- Empty auth → anonymous identity.
- Writes to `ctx` via `authctx.With(ctx, id)`.

The gateway reads identity from ctx instead of re-parsing the header. All current header parsing in `gateway.go` moves into the middleware; the gateway handler becomes oblivious to HTTP auth shape.

### 3. gRPC identity interceptor

`server/port/grpc/auth_interceptor.go` — unary + stream variants. Same extraction logic driven from `metadata.MD["authorization"]`. Installed in `server/main.go` alongside the existing interceptors.

### 4. Kill the bearer-UUID shortcut

Current: any UUID that hashes to an `AgentToken` row is accepted. Risk: UUIDs are predictable and these tokens have no prefix/version/checksum.

Replace with PASETO-signed agent tokens. Issuance (`/api/v1/auth/tokens` already exists) signs a paseto v4 local token with claims `{sub: agn_..., proj: prj_..., env: env_..., exp}`. Verification is O(1) signature check, no DB roundtrip.

Backwards-compat: keep a `legacy_uuid` path for 1 minor version (guarded by `cfg.Security.AllowLegacyTokens`, default false).

### 5. Vault credential resolver

`server/ops/auth/credresolve/vault.go`:

```go
type VaultResolver struct {
    client *vault.Client    // github.com/hashicorp/vault/api
    mount  string           // e.g., "secret/data/kave"
}

func (r *VaultResolver) Resolve(ctx, ref credresolve.Ref) (string, error) {
    secret, err := r.client.KVv2(r.mount).Get(ctx, ref.Path)
    ...
}
```

Wire into `credresolve.Resolve` based on `ConnectorCredential.Source`:

| Source       | Resolver          |
|--------------|-------------------|
| `env`        | env lookup (done) |
| `passthrough`| header forward (done) |
| `encrypted`  | keyring decrypt (done, verify) |
| `vault`      | new vault resolver |

Configuration via `cfg.Security.Vault.{Addr, Token, Mount}`. Omit section → `VaultResolver` not constructed → `vault` source returns `credresolve.ErrSourceDisabled`.

### 6. Casbin consolidation (hand-off to plan 11)

Two homes exist: `server/internal/infra/casbin/` and `server/ops/auth/casbin_engine.go`. Keep `internal/infra/casbin` (owns model + policy file loading + audit hook). Move the policy-evaluation wrapper from `ops/auth/casbin_engine.go` into the infra package and delete the duplicate. Plan 11 covers the broader dup cleanup; touch only the casbin files here.

### 7. Config

Extend `server/internal/config/types.go`:

```go
type Security struct {
    AllowAnonymous    bool
    AllowLegacyTokens bool
    SessionTTL        time.Duration
    TokenTTL          time.Duration
    Vault             *VaultConfig
}
```

Defaults: `AllowAnonymous=true` (preserves current event-mode DX), `AllowLegacyTokens=false`, `SessionTTL=24h`, `TokenTTL=30d`.

## Files

Create:
- `server/ops/auth/interceptor.go`, `interceptor_test.go`
- `server/internal/httpbridge/auth_middleware.go`, `auth_middleware_test.go`
- `server/port/grpc/auth_interceptor.go`
- `server/ops/auth/credresolve/vault.go`, `vault_test.go` (against a dev vault via testcontainers)

Modify:
- `server/main.go` — build auth interceptor, install HTTP middleware + gRPC interceptor, prepend auth to pipeline chain.
- `server/internal/gateway/gateway.go` / `routes.go` — read identity from `authctx`, drop inline header parsing.
- `server/internal/httpbridge/auth_routes.go` — token issuance signs PASETO; `login` returns session PASETO.
- `server/internal/config/types.go` — `Security` struct.
- `server/ops/auth/credresolve/resolve.go` — dispatch to vault resolver.
- `server/internal/infra/casbin/casbin.go` — absorb wrapper from `ops/auth/casbin_engine.go`; delete the latter.

## Acceptance

- `curl -X POST /v1/openai/chat/completions` without `Authorization` → 401 when `AllowAnonymous=false`.
- Same with `AllowAnonymous=true` → 200, traced under default agent.
- Valid PASETO agent token → 200.
- Agent token bound to agent X calling an endpoint forbidden by casbin policy for X → 403 `gateway.policy_blocked` (casbin path), **before** upstream call.
- Vault-sourced credential resolves live key for upstream request (integration test with `hashicorp/vault:latest` container).
- `ops/auth/casbin_engine.go` is deleted; all callers route through `internal/infra/casbin`.
- `go build ./... && go test ./...` clean.
- Load test: auth interceptor adds <150µs p99 per request on a warmed-up process.

## Out of scope
- OIDC / SSO (post-v1; cloud plan covers it).
- Per-org isolation in Enforce policy subjects (cloud).
- Token rotation UX (CLI plan).
- MFA.

## Size estimate
~900 LOC (interceptor 150, middleware 100, gRPC interceptor 80, vault 150, paseto issuance glue 100, casbin consolidation 150, config 40, tests 150). One haiku session.
