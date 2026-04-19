# Plan 06 — Auth, Sessions, Tokens, RBAC

**Goal:** replace the placeholder auth (bearer token = agent UUID lookup) with a real user/session/token + RBAC model. Re-enable the auth interceptor in the pipeline. All APIs enforce authorization via Casbin bindings. The framework gateway continues to accept agent-scoped tokens, but those tokens now live in a real store with revocation and expiry.

## Read first

- `core/ops/auth/auth.go` — existing primitives (keep, extend). `server/ops/auth/casbin_engine.go` — casbin wrapper (exists but not wired into the HTTP/gRPC path).
- `server/internal/gateway/gateway.go:51` — current "bearer UUID = agent id" shortcut. Keep the fast path but require the token to be a real `APIToken` record (not a raw agent id).
- `app/control/control.go` — control server already has token-ish methods around credentials; we're adding *identity* tokens distinct from connector credentials.
- `server/ops/auth/casbin_engine.go` — Casbin model file + adapter; we will add a store-backed adapter so policies live in the app store rather than on disk.

## Design

### 1. Identity model
New control models in `core/model/control/`:

```go
type User struct {
    ID, OrgID, Email, Name string
    Status string // "active"|"invited"|"disabled"
    PasswordHash []byte  // argon2id; empty if SSO-only
    CreatedAt, UpdatedAt int64
}

type Session struct {
    ID, UserID string
    TokenHash  []byte   // sha256 of bearer token; raw token never stored
    ExpiresAt  int64
    CreatedAt  int64
    LastUsedAt int64
    UserAgent  string
    IP         string
}

type APIToken struct {
    ID, OrgID, UserID string     // UserID optional — machine tokens have empty
    Name       string
    TokenHash  []byte
    Scopes     []string          // e.g. "agent:default", "policy:read", "*"
    ExpiresAt  *int64            // nil = no expiry
    LastUsedAt *int64
    CreatedAt  int64
    RevokedAt  *int64
}

type Role struct { ID, OrgID, Name string; Permissions []string /* casbin p lines */ }
type Binding struct { ID, OrgID, RoleID string; Subject string /* user:<id> | token:<id> */; Scope string /* project:env or "*" */ }
```

Migrations: `007_identity.up.sql` creates `users`, `sessions`, `api_tokens`, `roles`, `bindings` with indexes on `token_hash`, `user_id`, `org_id`. Cascade on user delete.

Store interfaces extend `store.AppStore` with CRUD + `LookupSessionByTokenHash`, `LookupAPITokenByHash`, `ListBindingsForSubject`.

### 2. Password + token hashing
`core/pkg/authhash/`:

- `HashPassword(pw) []byte` + `VerifyPassword(hash, pw) bool` — argon2id (`golang.org/x/crypto/argon2`).
- `HashToken(token) []byte` — sha256; deterministic (index on this column).
- `GenerateToken(prefix) (plain, hash)` — random 32 bytes, URL-safe base64, `"kpat_"` prefix for API tokens, `"ks_"` for session tokens.

### 3. Auth RPCs (control plane)

Extend `proto/control/auth.proto` with:

```
service AuthService {
  rpc Register(RegisterRequest) returns (Session);      // email + password, returns fresh session
  rpc Login(LoginRequest) returns (Session);
  rpc Logout(LogoutRequest) returns (google.protobuf.Empty);
  rpc ListSessions(ListSessionsRequest) returns (ListSessionsResponse);
  rpc RevokeSession(RevokeSessionRequest) returns (google.protobuf.Empty);
  rpc WhoAmI(WhoAmIRequest) returns (WhoAmIResponse);

  rpc CreateAPIToken(CreateAPITokenRequest) returns (CreateAPITokenResponse);  // returns plain once
  rpc ListAPITokens(ListAPITokensRequest) returns (ListAPITokensResponse);
  rpc RevokeAPIToken(RevokeAPITokenRequest) returns (google.protobuf.Empty);
}

service RBACService {
  rpc CreateRole / GetRole / ListRoles / UpdateRole / DeleteRole
  rpc CreateBinding / ListBindings / DeleteBinding
  rpc TestPermission(TestPermissionRequest) returns (TestPermissionResponse);
}
```

Implement in `app/control/auth.go` + `app/control/rbac.go`. Regenerate proto via existing `make` target.

### 4. Auth context + interceptor
`server/internal/authctx/`:

```go
type Principal struct {
    Kind   string  // "user"|"token"|"anonymous"
    UserID string
    TokenID string
    OrgID  string
    Scopes []string
}

func FromContext(ctx) (Principal, bool)
func WithPrincipal(ctx, Principal) ctx
```

**Unary gRPC interceptor** in `server/ops/auth/interceptor.go`:

1. Extract `Authorization: Bearer <token>` from incoming metadata / HTTP header (bridge copies header → metadata).
2. Hash, look up `Session` first, then `APIToken`. If either hit and not expired/revoked, build `Principal`.
3. Otherwise `Principal{Kind:"anonymous"}` — still allowed, but casbin will reject most actions.
4. Call casbin: `enforcer.Enforce(subject, object, action, scope)`. `subject = "user:<id>"|"token:<id>"|"anonymous"`, `object = <service.Method>`, `action = <verb>`, `scope = "<project>:<env>"` or `"*"`.
5. On deny: `codes.PermissionDenied` (→ bridge maps to 403).
6. Update `LastUsedAt` async (non-blocking goroutine, no await).

Plug the interceptor into `pipeline.New(authInterceptor, costInterceptor, traceInterceptor)` — **first** in order.

### 5. Casbin wiring
- Built-in model: RBAC with domains (`p, sub, obj, act, dom`).
- Adapter: store-backed (`Binding` rows → casbin policy lines on startup + `LoadPolicy()` on role/binding change). Avoid file adapter.
- Seed: on first boot, create role `admin` with `p, admin, *, *, *`. The first registered user gets bound to `admin` at `*:*`. Subsequent users get nothing until bound.
- Cache: keep the enforcer in memory; reload on mutation via `CasbinEngine.Reload()` called from the RBAC handlers.

### 6. Bridge routes
In `server/internal/httpbridge/routes.go` add for the new RPCs — they're unary so the existing bridge handles them. The `Authorization` header flows through; the interceptor on the gRPC side enforces.

Dedicated routes:
- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `GET  /api/v1/auth/whoami`
- `GET  /api/v1/auth/sessions`
- `DELETE /api/v1/auth/sessions/{id}`
- `POST /api/v1/auth/tokens` (create)
- `GET  /api/v1/auth/tokens`
- `DELETE /api/v1/auth/tokens/{id}`
- Full `/api/v1/rbac/roles{,/{id}}` and `/api/v1/rbac/bindings{,/{id}}` CRUD.
- `POST /api/v1/rbac/test` → permission check.

### 7. Gateway hardening
`server/internal/gateway/gateway.go`:

- Replace `isUUID(token)` shortcut with `APIToken` lookup. Token must have an `agent:<id>` scope (or `*`) to authorize as that agent.
- If no token: return 401 `auth.unauthenticated`. No more "anonymous request → default agent" behavior in prod. Keep an opt-in flag `gateway.allow_anonymous` (defaulting to **false**) that restores the default-agent fallback for local dev. Wire through config in Plan 05 shape.
- Emit `auth.login`/`auth.logout`/`token.created`/`token.revoked` events on the bus (Plan 03 hooks).

### 8. CLI flow sanity
`kave auth login` → `POST /api/v1/auth/login` → stores plain token in `~/.kave/credentials` (CLI owns this). CLI sends `Authorization: Bearer <token>` on every subsequent call. `kave auth token create --scope 'agent:default'` returns a plain token once — CLI writes it only to stdout.

## Files

Create:
- `core/model/control/{user,session,api_token,role,binding}.go`
- `core/pkg/authhash/hash.go`
- `server/internal/authctx/authctx.go`
- `server/ops/auth/interceptor.go`
- `server/ops/auth/seed.go` (admin role + first-user binding)
- `app/control/auth.go`, `app/control/rbac.go`
- `proto/control/auth.proto`, `proto/control/rbac.proto` (+ generated)
- Migrations: `007_identity.up.sql` / `.down.sql` for sqlite + postgres
- `server/ops/auth/interceptor_test.go`, `app/control/auth_test.go`

Modify:
- `server/internal/gateway/gateway.go` — token-based agent auth.
- `server/main.go` — build authInterceptor, put first in pipeline.
- `server/internal/httpbridge/routes.go` — add auth/rbac routes.
- `app/control/control.go` — accept Publish hooks already added in Plan 03.

## Acceptance

- `POST /api/v1/auth/register {email, password, name}` returns `Session` with plain token; subsequent unauthenticated call to `/api/v1/agents` returns 401; same call with `Authorization: Bearer <token>` succeeds.
- First registered user can `POST /api/v1/rbac/roles` and `POST /api/v1/rbac/bindings`. Second user cannot until bound.
- `POST /api/v1/auth/tokens {name, scopes:["agent:default"]}` → plain token; using it at the framework gateway authenticates as the default agent; using it against `/api/v1/policies` returns 403 (scope mismatch).
- `DELETE /api/v1/auth/sessions/{id}` invalidates the token on the next request.
- Interceptor refuses expired sessions/tokens.
- `TestPermission` matches the enforcer's decision.
- Anonymous framework-gateway calls: 401 by default; with `gateway.allow_anonymous: true` they fall back to default agent (preserves current dev UX behind a flag).
- `go build ./... && go test ./...` clean.

## Out of scope
- SSO / OAuth (post-v1).
- 2FA / WebAuthn (post-v1).
- Token rotation policy (we support revoke + recreate only).
- UI for role management (dashboard work).
- Audit log writes — Plan 08 covers audit storage; here we just emit events.

## Size estimate
~1200 LOC (proto + generated 300, models + store 250, hash/authctx 120, interceptor 200, handlers 250, tests 200). One large haiku session. If the agent's context runs thin, split between (a) identity + handlers and (b) interceptor + casbin + gateway — but attempt as a single session first.
