# Spec 02 — Integration Tests

Real stores, real wire, in-process server. Boundary tests that prove the
seams between modules hold under realistic data.

## 2.0 The harness — build this first

> **Post-plan-15 reality:** the control plane is gRPC-only. The harness exposes both an `*http.Client` (for the LLM gateway proxy + `/api/v1/fx/*` + `/health`) and a `*grpc.ClientConn` over `bufconn` (for everything else). No `httpbridge`.

**File:** `server/internal/testutil/harness.go`

```go
package testutil

type Harness struct {
    T          testing.TB
    Ctx        context.Context
    HTTP       *httptest.Server   // public + admin mux
    GRPC       *grpc.ClientConn   // bufconn dial
    App        store.AppStore     // sqlite :memory:
    Span       store.SpanStore    // duckdb temp file
    Audit      store.AuditStore   // sqlite :memory:
    Upstream   *MockUpstream      // configurable mock LLM
    Pipeline   *pipeline.Pipeline
    EncKey     []byte
    Identities IdentityFactory    // builds anon/user/agent contexts
    Now        func() time.Time   // injectable clock
}

func New(t testing.TB, opts ...Option) *Harness
```

**Options** (functional):
- `WithPostgres()` — boots a `testcontainers-go` postgres container, returns `AppStore` backed by it.
- `WithVault()` — boots `hashicorp/vault:latest`, dev mode, root token "root", initializes a kv-v2 mount at `secret/`.
- `WithFakeStore()` — uses `core/store/fake` (default; fastest).
- `WithFixedTime(t time.Time)` — pins `harness.Now` and the time helpers passed to ops packages.
- `WithSeed(seed *Seed)` — pre-populates org/user/project/env/agent/policy/budget. `Seed` exposes the IDs.
- `WithFrameworkRegistry(r *gateway.Registry)` — override default.
- `WithPolicy(model, policy string)` — install a casbin model+policy for the auth interceptor.

**Lifecycle:** `t.Cleanup` shuts down httptest, gRPC server, all stores, all containers. Containers are reused across the package via `sync.Once` + a `TestMain` helper to amortise the ~3s boot.

**MockUpstream** (`testutil/mock_upstream.go`):
- `*httptest.Server` with a router matching real provider paths (`/v1/chat/completions`, `/v1/messages`, `/v1beta/...`).
- Per-test fixtures: `mock.OnPath("/v1/chat/completions").RespondJSON(http.StatusOK, body)` and `mock.OnPath(...).StreamSSE(events []string, gapMs int)`.
- Counters: `mock.RequestCount(path)` for "upstream never called" assertions.
- Headers captured: `mock.LastRequest(path).Header.Get("Authorization")` for credential-resolution tests.

**Identity factory** (`testutil/identity.go`):
- `Identities.Anonymous(ctx) context.Context`
- `Identities.User(ctx, userID, agentID string) context.Context`
- `Identities.AgentToken(ctx, agentID string) context.Context`
- Builds `authctx.Identity` + injects via `authctx.With`.

**Eventually helper** (`testutil/eventually.go`):
- `Eventually(t, 2*time.Second, 20*time.Millisecond, func() bool { ... }, "msg")`. No `time.Sleep` in tests — only this.

**Coverage of the harness itself:** `harness_test.go` boots one with each option and asserts the basic invariants (HTTP returns 200 on `/healthz`, gRPC `Ping` returns OK, store has the seed rows).

## 2.1 The shared store suite — build this second

**File:** `core/store/storetest/suite.go`

A package-level set of test functions that take a `func(t *testing.T) store.AppStore` (and one for `SpanStore`, one for `AuditStore`) and exercise every method of the interface against the live impl. Each backend test file is then a thin wrapper:

```go
// server/internal/store/sqlite/app_store_test.go
func TestSQLiteApp(t *testing.T) {
    storetest.RunAppStoreSuite(t, func(t *testing.T) store.AppStore {
        return newSQLiteAppForTest(t)  // helper local to the package
    })
}
// server/internal/store/postgres/app_store_test.go - same shape, testcontainers PG
// server/internal/store/duckdb/span_store_test.go - same shape
```

**AppStore suite must cover, per interface group:**

- `OrgStore`: create unique slug, get by id+slug, list pagination roundtrip, duplicate slug → wrapped `core/store.ErrConflict` (introduce sentinel if missing).
- `UserStore`: create + get, get by email scoped to org, update partial (only nil-able fields touched).
- `MembershipStore`: add, get, list with `Page{Limit:1}` returns one + `NextCursor`, remove makes `GetMembership` return `ErrNotFound`.
- `ProjectStore`, `EnvironmentStore`: as above; `GetEnvironmentBySlug` is project-scoped.
- `AgentStore`: create + get + list; **soft-delete** sets `DeletedAt` and excludes from list; `RestoreAgent` brings it back; double-delete is idempotent.
- `PolicyStore`: `GetAgentPolicy` returns the policy bound to an agent; updating the policy doesn't change ID; delete unbinds.
- `BudgetStore`: create then get; second create for same agent → `ErrConflict`; delete is idempotent.
- `RunStore`: create + get; idempotency-key lookup; status transitions (`Active → Completed/Failed`); list filtered by env, agent, status, time range.
- `ActionStore`: create + get + list-by-run paginated.
- `CostStore`: save then get price book; FX rate upsert (idempotent); `InsertBudgetEntry` then `SumAgentSpend(sinceMs=0)` returns the sum; `AddRunSpend` accumulates; `GetSpendReport` group-by combinations (`day`, `agent`, `provider`, `model`).
- `TokenStore`: insert session/agent/api token, get-by-hash, revoke makes get return `ErrRevoked` (or filtered out — pin), touch updates `LastUsedAt`.
- `CredentialStore`: store, resolve fallback chain (exact label → "primary" → any active), rotate (new blob, version++), revoke, touch — never returns the encrypted blob in a list.
- `RoleStore` / `BindingStore`: CRUD + list pagination.
- `StoreLifecycle`: `WithTx` rolls back on returned error and on panic; `Migrate` is idempotent (run twice without error).

**SpanStore suite:**
- `OpenSpan` then `CloseSpan` updates end fields; `GetSpan` returns merged row.
- `QuerySpans` with each filter from `SpanFilter` (trace_id, agent_id, env_id, status, time range, attribute equality).
- `SpendByDimension` for `provider`, `model`, `agent_id`, `day` — sums match insertions.
- Closing an unknown span → `ErrNotFound`.
- Concurrent `OpenSpan` (100 goroutines) — no duplicate-key, all 100 readable.

**AuditStore suite:**
- Append then query by actor, action, resource, time range.
- Append is append-only: no update API; Query returns events in `created_at DESC` order (or pin actual order).
- Pagination roundtrip across 1000 entries.

**Cross-store parity:** `TestSpanStoreParity` runs the same scenarios against duckdb and postgres in subtests, then `cmp.Diff` the result sets. Surfaces dialect bugs.

## 2.2 Gateway happy path

**File:** `server/internal/gateway/gateway_integration_test.go`

```
func TestGateway_OpenAI_Buffered_HappyPath(t *testing.T)
```

Setup: `harness.New(t, WithSeed(seed.WithAgentAndCredential("openai", "sk-test")))`.
Mock upstream returns OpenAI-shaped response with usage `{prompt_tokens: 100, completion_tokens: 50}`.

Steps:
1. `POST /v1/openai/chat/completions` with agent token in `Authorization: Bearer agt_...`.
2. Wait for the run to finish (use `Eventually` on `app.GetRunByID(runID).Status == "completed"`).

Assertions (all four — plan 13 line 50):
- HTTP body equals upstream body (byte-for-byte).
- HTTP status is 200; `Content-Type` mirrored.
- `app.ListActions(runID)` has exactly 1 action with `Status == "completed"`.
- `app.GetRunByID` shows `EndedAt > StartedAt`, `Status == "completed"`.
- `app.SumAgentSpend(agentID, 0) > 0` — exact value computed from the fixture's price book.
- A budget entry row exists with `Cost == expected`, `InputTokens == 100`, `OutputTokens == 50`, `PriceVersion == seed.PriceBook.Version`.
- `mock.LastRequest("/v1/chat/completions").Header.Get("Authorization") == "Bearer sk-test"` (proves credential resolution).

## 2.3 Gateway streaming (SSE)

**File:** same as 2.2.

```
func TestGateway_Streaming_OrderAndUsage(t *testing.T)
```

Mock upstream: `mock.OnPath("/v1/chat/completions").StreamSSE(events, 5*time.Millisecond)` where `events` is 20 chunks ending with a `data: [DONE]` and a usage frame.

Assertions:
- Client receives 20 chunks in order. Use `bufio.Scanner` over the response body, collect prefixed lines, compare to `events`.
- Last chunk delivered before `httpResp.Body.Close()` returns.
- Span closed (`SpanStore.GetSpan(spanID).EndedAt != 0`) within 200ms after last chunk — assert via `Eventually`.
- Cost from accumulated usage matches `Calculate(snapshot, usage)`.

Edge cases:
- Upstream closes mid-stream (mock returns `n, io.ErrUnexpectedEOF` after 5 chunks): pipeline still records 5 chunks of partial usage, span marked failed, run ends `failed` with non-empty `ErrorMessage`. Client sees the 5 chunks then the connection closes — no panic.
- Client disconnects mid-stream: simulated by cancelling the request context. Server stops writing within 100ms (no goroutine leak — count goroutines before/after).

## 2.4 Policy block

**File:** same.

`TestGateway_PolicyBlock_403`:
- Seed casbin with deny rule for the test agent on `gateway.openai.chat.completions`.
- POST same endpoint.
- Assert: HTTP 403, body contains `"code":"gateway.policy_blocked"`, `details.subject` and `.object` populated.
- Assert: `mock.RequestCount("/v1/chat/completions") == 0` (upstream never called).
- Assert: `auditStore.QueryAudits(filter:{action:"gateway.policy_blocked"})` returns 1 row.
- Assert: `app.SumAgentSpend(agentID, 0) == 0`.

## 2.5 Budget block

`TestGateway_BudgetBlock_402`:
- Seed agent with `MonthlyBudget = $0.10`; pre-load budget entries totalling `$0.10`.
- POST.
- Assert: HTTP 402, body code `gateway.budget_exceeded`, `details.spent`, `.limit`, `.period == "monthly"`.
- Assert: upstream never called.
- Assert: a zero-cost audit row written (action `gateway.budget_exceeded`).

## 2.6 Auth round-trip (gRPC)

**File:** `server/port/grpc/auth_integration_test.go`

`TestAuth_Login_Session_Bearer`:
1. Call `AuthService.Login` (gRPC) with seeded user creds → returns PASETO session token.
2. Call `AgentService.List` over a connection that injects `authorization: Bearer <token>` metadata → OK, response lists the seeded agent.
3. Same call with truncated token → `codes.Unauthenticated` with detail code matching the auth interceptor's mapping.
4. Revoke session via store; same call → `codes.Unauthenticated`.
5. PASETO `kind="user"` token used for an agent-only RPC → `codes.PermissionDenied`.

Use the harness's `Identities` factory + grpc per-RPC credentials to attach the metadata.

## 2.7 Vault credential resolve

**File:** `server/ops/auth/credresolve/vault_integration_test.go`

`TestVault_ResolvesUpstreamKey` (uses `harness.New(t, WithVault())`):
1. Write to vault: `vaultClient.KVv2("secret").Put(ctx, "openai", map[string]any{"api_key": "sk-from-vault"})`.
2. Seed credential with `Source = vault`, `Reference = "secret/openai#api_key"`.
3. POST to `/v1/openai/chat/completions`.
4. Assert: `mock.LastRequest("/v1/chat/completions").Header.Get("Authorization") == "Bearer sk-from-vault"`.
5. Negative: bad vault path → resolve returns wrapped error; gateway returns 500 with `code: gateway.internal_error`; no upstream call.
6. Negative: vault token expired → wrapped error reports vault status.

## 2.8 Apply engine

**File:** `server/internal/daemon/apply_integration_test.go` (extend existing `apply_test.go`).

`TestApply_CreatesAndDiffsClean`:
1. Build a `kave.yaml` fixture: 3 agents, 2 policies (one binding agent→policy).
2. `apply.Run(ctx, fixture)` → returns no error, `ApplyResult` lists 3 agent creates + 2 policy creates.
3. Re-run `apply.Diff(ctx, fixture)` → empty diff (`len(create)+len(update)+len(delete) == 0`).
4. Mutate fixture (rename one agent description) → diff lists one update with the changed field path.
5. Remove an agent from fixture → diff lists one delete; apply with `--prune` removes it; without `--prune` keeps it.

## 2.9 Trace tree RPC

**File:** `server/app/runtime/trace_integration_test.go` (or wherever the gRPC `RuntimeService.GetTrace` handler lives — adjust on read).

`TestTraceTree_Returns_NestedShape`:
1. Synthesize 7 spans (root → 2 mid → 4 leaves), insert via `SpanStore.OpenSpan`+`CloseSpan` directly.
2. Call `RuntimeService.GetTrace(trace_id=...)` over the harness gRPC client.
3. Compare the proto response to a hand-built expected tree using `cmp.Diff` with `protocmp.Transform()`. Span timing fields ignored via `protocmp.IgnoreFields`.
4. Negative: unknown trace ID → `codes.NotFound`.
5. Negative: build error (orphan span injected by bypassing OpenSpan) → `codes.Internal`; one `level=error` log line with `trace_id` attr captured via the buffer slog handler.

## 2.10 gRPC streaming fanout (post-plan-15)

**File:** `server/app/runtime/streams_integration_test.go` (or wherever the streaming RPC lives — `kave.runtime.v1.SpanStream` or equivalent. The HTTP bridge is gone; streams are gRPC server-streaming RPCs. Read `server/port/grpc/` to confirm the exact service+method name, then update the file path).

`TestSpanStream_10Subscribers_OrderedDelivery`:
1. Open 10 concurrent gRPC streaming clients (each calls the streaming RPC over the harness's `bufconn`).
2. Producer publishes 100 distinct events on the bus (e.g., `{seq: i}`).
3. Each subscriber collects events into a slice via `stream.Recv()`.
4. Assert: every subscriber received all 100 events; per-subscriber slice equals `[0..99]` in order.
5. Drop one slow subscriber (read at 1ms intervals while others read freely): no head-of-line blocking — fast subscribers still finish in <1s.
6. Cancel one subscriber mid-stream (`cancel()` on the client ctx): server-side goroutine count returns to baseline within 500ms (`runtime.NumGoroutine`).
7. Backpressure: subscriber that never calls `Recv()` should not stall the publisher — assert via a deadline; document the drop policy (drop oldest vs disconnect) chosen in the implementation.

Note on the LLM gateway proxy (`/v1/openai`, `/v1/anthropic`, `/v1/google`, `/frameworks/*`) and `/api/v1/fx/*`: those remain HTTP by design (gateway proxy is HTTP-native; fx is a small public utility). All control-plane streaming is gRPC after plan 15.

## 2.11 Cross-store parity (covered by 2.1)

Verified by `RunSpanStoreSuite` running against duckdb and postgres backends.

---

## Wiring testcontainers

`server/internal/testutil/containers.go`:
- `postgresContainer(t)` → `postgres:16-alpine`, exposes port, returns DSN, runs `Migrate` against it.
- `vaultContainer(t)` → `hashicorp/vault:latest`, dev mode, returns address + root token.
- Both reuse via `sync.Once` keyed by image name; closed in a `TestMain`-registered cleanup.

CI flag: `INTEGRATION=1` env gates these tests via `if os.Getenv("INTEGRATION") != "1" { t.Skip() }`. The Makefile's `test-integration` sets it; default `make test` runs unit only.

## What an implementing agent should produce

1. The harness package + tests for it.
2. The `storetest` package + per-backend test files.
3. One `_integration_test.go` per scenario above.
4. `containers.go` helper.
5. Updated `Makefile` targets `test-unit`, `test-integration`.
6. Run `INTEGRATION=1 go test -race ./...` locally; report timings and any flakes in the PR.
