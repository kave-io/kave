# Plan 06 — Auth, Sessions, Tokens, RBAC, Credential Sources

**Goal:** implement the auth model from `docs/src/content/docs/concepts/10-authentication.md`. Three separate concerns, never conflated:

1. **User → daemon** — humans/CI authenticate with session tokens stored in `~/.kave/`.
2. **Agent → daemon** — running agents authenticate with agent tokens.
3. **Agent → external service** — outbound credentials. Kave holds a *reference*, not the value. Four sources: `env`, `vault`, `passthrough`, `encrypted` (local dev only).

Re-enable the auth interceptor so every API call resolves to a `Principal` and Casbin enforces. No secrets API — credentials are opaque; the value cannot be read back.

## Read first

- `docs/src/content/docs/concepts/10-authentication.md` — ground truth. Key philosophy: Kave knows *which* credential and *whether* a policy permits it; never serves the value.
- `core/ops/auth/auth.go`, `server/ops/auth/casbin_engine.go` — existing primitives to extend.
- `server/internal/gateway/gateway.go:51` — current placeholder (`isUUID(token) → agent`). Replace with real agent-token lookup + credential source dispatch.
- Existing `ConnectorCredential` model — extend; don't replace. Today it stores a value; we're adding `Source` + per-source fields and removing value access from the API.

## Design

### 1. Identity model (concerns 1 & 2)

New `core/model/control/`:

```go
type User struct {
    ID, OrgID, Email, Name string
    Status       string    // active | invited | disabled
    PasswordHash []byte    // argon2id; empty when SSO-only
    CreatedAt, UpdatedAt int64
}

type Session struct {           // concern 1: user → daemon
    ID, UserID string
    TokenHash  []byte           // sha256(raw); raw never persisted (doc §"never stored in plaintext")
    ExpiresAt, CreatedAt, LastUsedAt int64
    UserAgent, IP string
}

type AgentToken struct {        // concern 2: agent → daemon
    ID, OrgID, AgentID string
    Name       string
    TokenHash  []byte
    Scopes     []string          // e.g. "agent:<id>", "connector:anthropic"
    ExpiresAt  *int64
    LastUsedAt *int64
    CreatedAt  int64
    RevokedAt  *int64
}

type APIToken struct {          // concern 1: machine user → daemon (for CI/scripts)
    ID, OrgID, UserID string
    Name       string
    TokenHash  []byte
    Scopes     []string
    ExpiresAt, RevokedAt *int64
    LastUsedAt *int64
    CreatedAt  int64
}

type Role    struct { ID, OrgID, Name string; Permissions []string }   // casbin p-lines
type Binding struct { ID, OrgID, RoleID string; Subject, Scope string } // subject: user:<id>|token:<id>|agent:<id>
```

**AgentToken is distinct from APIToken** — same shape, different lifecycle. Agents don't log in; an operator mints an AgentToken and hands it to the process.

Migrations: `007_identity.up.sql` creates `users`, `sessions`, `agent_tokens`, `api_tokens`, `roles`, `bindings`. Unique index on each `token_hash` column. Cascade on user/agent delete.

### 2. Credential sources (concern 3)

Extend `core/model/control/ConnectorCredential`:

```go
type Credential struct {
    ID, ProjectID, EnvID, Name, Connector string

    Source CredentialSource  // env | vault | passthrough | encrypted

    // Source-specific (exactly one group populated):
    EnvVar    string                 // source: env
    VaultRef  string                 // source: vault — e.g. "kv/data/kave/gh-prod#token"
    Encrypted *EncryptedBlob         // source: encrypted (local dev only)
    // passthrough: no fields — caller supplies header at request time.

    Metadata map[string]any
    CreatedAt, UpdatedAt int64
    RevokedAt *int64
}

type EncryptedBlob struct {
    Ciphertext []byte  // AES-256-GCM
    Nonce      []byte
    KeyID      string  // keyring entry identifier
}
```

**Resolver** in `server/ops/auth/credresolve/`:

- `Resolve(ctx, cred) (string, error)` — returns the raw secret *only* to internal call sites (interceptors/gateway), never through an API handler.
- `env`: `os.Getenv(cred.EnvVar)`; error if unset.
- `vault`: pluggable `VaultClient` interface; ship a no-op default that returns `codes.Unimplemented` unless configured (real HashiCorp/AWS/GCP adapters are post-v1 work, but the interface exists so the code path is wired).
- `passthrough`: returns `""` + a sentinel `ErrPassthrough` that signals the gateway to forward the caller's `Authorization` header untouched.
- `encrypted`: decrypt via `keyring` package; on `ErrKeyringUnavailable`, daemon startup and `kave doctor` both flag it (doc §"daemon refuses to start").

**No GET endpoint exposes the resolved value.** `/api/v1/credentials/{id}` returns metadata only. `POST /api/v1/credentials/{id}/test` calls Resolve + a light upstream probe and returns `{ok, latency_ms, error}` — never the secret.

### 3. Keyring integration (encrypted tier)

`core/pkg/keyring/keyring.go` — thin wrapper over `github.com/zalando/go-keyring` (cross-platform: Keychain / GNOME / DPAPI).

- `GetOrCreateMasterKey(ctx) ([]byte, error)` — 32-byte key, created on first use.
- Daemon startup: if any `credential.source == encrypted` exists, call `GetOrCreateMasterKey`; hard-fail on error.
- `kave doctor` check `credentials_resolve` extends: for each `encrypted` credential, attempt unwrap; any failure → `fail`.

### 4. Password + token hashing

`core/pkg/authhash/`:

- `HashPassword(pw) []byte` / `VerifyPassword(hash, pw) bool` — argon2id.
- `HashToken(plain) []byte` — sha256 (deterministic, so `token_hash` can be indexed). Doc explicitly specifies SHA-256 for tokens.
- `GenerateToken(prefix) (plain, hash)` — 32 random bytes, URL-safe base64. Prefixes: `ks_` (session), `kpat_` (user API token), `kat_` (agent token).

### 5. Auth RPCs

Extend `proto/control/` with two services:

```proto
service AuthService {
  // user → daemon
  rpc Register, Login, Logout, ListSessions, RevokeSession, WhoAmI, ChangePassword
  rpc CreateAPIToken, ListAPITokens, RevokeAPIToken

  // agent → daemon
  rpc CreateAgentToken, ListAgentTokens, RevokeAgentToken
}

service RBACService {
  rpc CreateRole, GetRole, ListRoles, UpdateRole, DeleteRole
  rpc CreateBinding, ListBindings, DeleteBinding
  rpc TestPermission
}
```

Implement in `app/control/auth.go` + `app/control/rbac.go`. Each `Create*Token` returns the **plain token once** (doc §"raw token is shown once at creation"); subsequent GETs return metadata only.

Every mutation emits a bus event (`auth.login`, `auth.logout`, `token.created`, `token.revoked`, `credential.created`, `credential.revoked`) **and** writes an append-only audit row via `app/audit` (doc §"Every mutation is audited"). Existing `core/model/audit` already has the shape — wire the writer into each handler.

### 6. Auth context + interceptor

`server/internal/authctx/`:

```go
type Principal struct {
    Kind    string   // user | api_token | agent | anonymous
    OrgID   string
    UserID  string   // when Kind in {user, api_token}
    AgentID string   // when Kind == agent
    TokenID string
    Scopes  []string
}
func FromContext(ctx) (Principal, bool)
func WithPrincipal(ctx, Principal) context.Context
```

Unary gRPC + bridge interceptor (`server/ops/auth/interceptor.go`):

1. Extract `Authorization: Bearer <token>`. Hash with `HashToken`.
2. Look up in order: `Session` → `APIToken` → `AgentToken`. First hit wins; check expiry+revocation.
3. Build `Principal`. No token → `Kind: anonymous`.
4. Casbin enforce: `subject = <Kind>:<ID or "anonymous">`, `object = <service.Method>`, `action = <verb>`, `scope = "<project>:<env>"` or `"*"`.
5. Deny → `codes.PermissionDenied` (bridge → 403).
6. Fire-and-forget `UPDATE ... SET last_used_at = ?`.

Pipeline order: `pipeline.New(authInterceptor, costInterceptor, traceInterceptor)` — auth first.

### 7. Casbin wiring

- Model: RBAC with domains (`p, sub, obj, act, dom`).
- Adapter: store-backed — `Binding` rows become casbin policy lines at boot and on any role/binding mutation. No file adapter.
- Seed: built-in `admin` role (`p, admin, *, *, *`). First user who registers gets bound to `admin` at `*:*`. Later users get nothing until bound.
- `CasbinEngine.Reload()` called from RBAC handlers after writes.

### 8. Gateway hardening

`server/internal/gateway/gateway.go`:

- Remove `isUUID(token)` shortcut.
- New flow:
  1. Extract bearer token → look up `AgentToken` → derive `agentID` + scopes.
  2. Select the `Credential` named by the agent's policy for the target connector.
  3. Call `credresolve.Resolve`:
     - On `ErrPassthrough`: forward the caller's original `Authorization` header to the upstream as-is.
     - Otherwise: inject the resolved secret into the upstream-connector-specific header (`x-api-key` for Anthropic, `Authorization: Bearer` for OpenAI, etc. — per existing connector code).
  4. **Never** log or trace the resolved secret. Add a test that captures the log+span output and asserts the secret string never appears.
- Config flag `gateway.allow_anonymous` (default `false`): restores the current default-agent fallback for local dev. When true and no token present, proceeds as `agent:default`. Log a startup WARN when enabled.

### 9. Bridge routes

Add to `server/internal/httpbridge/routes.go`:

- `POST /api/v1/auth/register`, `/login`, `/logout`, `/change-password`
- `GET  /api/v1/auth/whoami`
- `GET|DELETE /api/v1/auth/sessions[/{id}]`
- `POST|GET /api/v1/auth/tokens` (user API tokens); `DELETE /api/v1/auth/tokens/{id}`
- `POST|GET|DELETE /api/v1/auth/agent-tokens[/{id}]`
- `POST|GET|DELETE /api/v1/rbac/roles[/{id}]`
- `POST|GET|DELETE /api/v1/rbac/bindings[/{id}]`
- `POST /api/v1/rbac/test`
- `POST /api/v1/credentials/{id}/test` — source probe; returns `{ok,latency_ms,error}`. **No** `GET .../value`.

### 10. Config surface (bridges into Plan 05 shape)

```yaml
credentials:
  - name: anthropic-prod
    connector: anthropic
    source: env
    env: ANTHROPIC_API_KEY

  - name: github-prod
    connector: github
    source: vault
    ref: kv/data/kave/gh-prod#token

  - name: caller-openai
    connector: openai
    source: passthrough

gateway:
  allow_anonymous: false
```

The loader validates that `source` ∈ {env, vault, passthrough, encrypted} and that the right per-source field is set. `encrypted` credentials cannot be declared in `kave.yaml` — only minted via `kave credential create --source encrypted` (interactive prompt). The loader rejects encrypted entries in YAML with a clear error.

## Acceptance

- Fresh daemon; `POST /api/v1/auth/register` → returns plain session token; `/api/v1/agents` with that token = 200, without = 401.
- First user can create roles + bindings. A second registered user gets 403 on `/api/v1/rbac/roles` until bound.
- `POST /api/v1/auth/tokens {scopes:["agent:default"]}` returns plain token once; reusing it against `/api/v1/policies` is 403.
- `POST /api/v1/auth/agent-tokens` yields a token the framework gateway accepts. Without the token: 401 (or default-agent fallback only if `gateway.allow_anonymous=true`).
- Credential with `source: env, env: OPENAI_API_KEY` — unset the env var, `POST /api/v1/credentials/{id}/test` returns `{ok:false, error:"env var unset"}`. Set it, test returns `{ok:true}`.
- Credential with `source: passthrough` — gateway forwards the caller's own `Authorization` header untouched to the upstream.
- `GET /api/v1/credentials/{id}` payload never contains the raw value, env-var value, or vault ref contents.
- Log capture test: make a gateway call with a known secret in the env; grep the captured log/span output — secret must not appear.
- `encrypted` credential on a machine with the keyring unreachable: daemon startup fails with a clear "keyring unavailable" error; `kave doctor` shows `credentials_resolve: fail` with the same cause.
- Every token/credential create/revoke writes one audit row and emits one bus event.
- `DELETE /api/v1/auth/sessions/{id}` invalidates the token on the next request.
- `go build ./... && go test ./...` clean across modules.

## Out of scope
- SSO / OIDC / WebAuthn (doc §"not ever, as a matter of design" for SSO for external services; user SSO is post-v1).
- Real Vault adapter implementations — interface only, stub returns `Unimplemented`.
- Rotation flows — revoke + recreate only.
- `file:` credential source (doc mentions it in passing; not in the enumerated list; defer).
- Audit log UI / query endpoints — Plan 08.
- Dashboard RBAC screens.

## Size estimate
~1400 LOC:
- proto + generated: 350
- models + migrations + store: 300
- credresolve + keyring: 180
- authhash + authctx: 120
- interceptor + casbin adapter: 220
- handlers (auth + rbac + credential.test): 200
- tests: 200 (interceptor, resolver, gateway no-leak test, audit-row assertion)

One long haiku session. If context tightens, split as: (a) identity + RBAC + interceptor, (b) credential sources + gateway hardening + keyring. Attempt single-session first.
