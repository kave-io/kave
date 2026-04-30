# Design: Auth Boundary — When Kave Requires Auth

**Status:** Locked v1 design. Implements decision #5 in `v1_decisions.md`.
Replaces the `allowAnonymous` boolean on `FrameworkGateway`.

## Problem

The current `allowAnonymous` is a single boolean on the gateway. It does
not capture the real product question:

- Some upstream providers don't need credentials (local Ollama, free
  llama.cpp, model-mesh deployments). For those, requiring Kave auth on
  the inbound side adds friction without security benefit.
- Some deployments are local-only dev sandboxes. Requiring auth makes
  `curl` examples in the README tedious.
- Most deployments are production. Anonymous access there is a CVE.

The fix isn't a knob. It's three orthogonal axes that the gateway
evaluates jointly.

## Three axes

### Axis 1 — Environment trust mode

Every `Environment` carries a `TrustMode` enum:

```go
type TrustMode string

const (
    TrustStrict     TrustMode = "strict"     // default — auth required
    TrustPermissive TrustMode = "permissive" // anonymous allowed when other axes also permit
)
```

- New environments default to `strict`.
- `kave init` creates a `dev` environment with `trust_mode: permissive`
  to preserve dev ergonomics.
- Production deployments stay `strict` — the operator must explicitly
  flip it via `kave apply` to relax. The flip is logged at `level=warn`
  and audited.

### Axis 2 — Provider auth requirement

Each `LLMConnector` declares whether the upstream needs credentials:

```go
type LLMConnector interface {
    // ...existing methods...
    RequiresAuth() bool
}
```

- `openai`, `anthropic`, `google`, `groq`, `mistral` → `true`.
- `ollama`, `llamacpp`, `local-mesh` → `false`.
- New connectors must implement this; the lint rule `B5-connector-iface`
  asserts the method exists on every registered connector.

### Axis 3 — Bind scope

The server's listening address determines how strict the network surface
is:

- Loopback (`127.0.0.1`, `::1`) — least privileged callers, dev-shaped.
- Non-loopback — server is reachable from other hosts; production-shaped.

A configuration check at startup: if **any** environment has
`trust_mode: permissive` **and** the server is bound to a non-loopback
address, refuse to start unless `KAVE_ALLOW_PERMISSIVE_PUBLIC=1` is set.
The error message says exactly that, with the offending env names listed.

## The gateway decision (one place)

Anonymous is accepted **iff all three** are true:

1. The target environment's `TrustMode == TrustPermissive`.
2. The resolved `LLMConnector.RequiresAuth() == false`.
3. The server bind is loopback **or** `KAVE_ALLOW_PERMISSIVE_PUBLIC=1`
   was set at startup (this part is a startup-time check, not per-request).

If any axis fails, the request is rejected with **the most specific**
error code so the operator can fix the right thing:

| Failing axis | Status | Code |
|---|---|---|
| 1 (strict env) | 401 | `auth.env_requires_authentication` |
| 2 (paid provider) | 401 | `auth.provider_requires_authentication` |
| 3 (caught at startup) | server refuses to boot | n/a |

## The synthetic guest identity

When all three axes permit, the request is processed under a synthetic
identity:

```go
authctx.Identity{
    Kind:        authctx.KindGuest,   // new kind
    EnvID:       <env-id>,
    Connector:   <connector-name>,
    AgentID:     "",                  // no agent — guest has none
    OrgID:       <env's org>,
    BindScope:   "loopback" | "public",
}
```

Properties of `KindGuest`:

- **No agent binding.** The seeded "default" agent is removed from v1.
  `gateway.handleProxy` no longer falls back to `agentID := "default"`.
- **Zero budget.** `policy.CostPolicy` for a guest identity always returns
  `BudgetCap: $0`. Any non-zero-cost action is rejected
  (`gateway.budget_exceeded` — same code path as exhausted budget).
  Free providers can record actions because their cost is genuinely 0.
- **Casbin subject.** The casbin subject string is `guest:<connector>` so
  the policy file can express things like
  `p, guest:ollama, /v1/local/*, POST, allow`.
- **Audited.** Every guest invocation writes an audit row with
  `actor: "guest"`, `actor_detail: <bind-scope>`. Operators can grep for
  guest traffic.
- **Rate-limited.** A per-IP token bucket lives at the gateway: 10 RPS
  per `connector` per `remote_addr`, configurable. Guests hitting the
  bucket get 429.

## Removing `default-agent` seeding

`server/main.go` currently seeds a "default" agent on startup so the
gateway can fall back to it. With guests replacing that path, the
fallback goes away. The seed is replaced with:

- A bootstrap script that creates a single `dev` env with `trust_mode:
  permissive`, no default agent, and an example `kave.yaml` showing how
  to add one.
- The smoke-test path becomes: `kave start --dev` → boots with the dev
  env preconfigured for guest access against `ollama` only. Documented in
  the README.

## Configuration shape

In `kave.yaml`:

```yaml
environments:
  - name: dev
    trust_mode: permissive
  - name: prod
    trust_mode: strict     # default; explicit for clarity
```

In env: `KAVE_ALLOW_PERMISSIVE_PUBLIC=1` (server-level override; the only
thing that allows non-loopback + permissive).

## Test plan additions

Add to spec 02:

- `TestAuth_StrictEnv_AnonymousRejected` — strict env + Ollama → 401
  `auth.env_requires_authentication`.
- `TestAuth_PermissiveEnv_PaidProvider_AnonymousRejected` — permissive
  env + OpenAI → 401 `auth.provider_requires_authentication`.
- `TestAuth_PermissiveEnv_FreeProvider_GuestAllowed` — permissive env +
  Ollama → 200, identity = guest, audit row written, agent_id empty.
- `TestAuth_GuestBudget_BlocksNonZeroCost` — synthetic non-zero cost
  recorded → 402 `gateway.budget_exceeded` on the next call.
- `TestStartup_PermissiveOnPublicBind_Refuses` — bind to `0.0.0.0`,
  permissive env, no override → server fails to start with descriptive
  error.
- `TestStartup_PermissiveOnPublicBind_AllowedWithOverride` — same +
  `KAVE_ALLOW_PERMISSIVE_PUBLIC=1` → starts, logs a banner.

Add to spec 02 audit suite:

- Every guest call appears in `AuditStore` with `actor: "guest"`.

## Migration / cleanup

In the cleanup PR (before tests start):

1. Remove `allowAnonymous bool` from `FrameworkGateway`.
2. Remove the seed of "default" agent in `server/main.go`.
3. Replace `agentID := "default"` fallback with the three-axis decision.
4. Add `TrustMode` field to `Environment` (with migration default
   `strict` for existing rows).
5. Add `RequiresAuth() bool` to the `LLMConnector` interface; implement
   on every registered connector.
6. Add the gateway rate limiter (token bucket per connector × remote
   addr).
7. Add the startup public-bind check.

## What this prevents

- A misconfigured production env can't accidentally serve anonymous
  paid-provider traffic — axis 2 blocks it even if axis 1 is wrong.
- An attacker who finds the public address of a permissive deployment
  can't get free OpenAI calls — axis 2 blocks it.
- An operator who relaxes one env can't accidentally relax all of them
  — `TrustMode` is per-env.
- A new free-provider connector can't accidentally require auth — its
  author has to make a decision in the interface implementation, and the
  policy file has to allow `guest:<that-connector>`.

The blast radius for any single mistake is bounded by the AND of three
axes.
