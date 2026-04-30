# Architecture Boundaries — Kave v1

These are the durable laws of the codebase. They are enforced by the
architecture linter (`cmd/lint-architecture`, see `03-architecture-lint.md`),
which runs in CI as a normal `go test`. Adding or weakening a rule requires
updating this file and the linter together.

Every rule has: **what**, **why**, **how it's enforced**, and **how to
override** (when overriding is allowed).

---

## B1. Layer boundary — proto / model / runtime

**What.** Three layers, one direction of dependency:
- `proto/kave/*` is the wire contract. It imports nothing from `core/`.
- `core/model/*` is durable storage shapes. It does **not** import
  `core/runtime/*`. (The reverse is fine.)
- `core/runtime/*` is in-memory live state. It may import `core/model/*` and
  `proto/*` only via the mapper layer.
- `core/mappers/*` is the only place that imports both `core/runtime/*` and
  `proto/*` (and may also import `core/model/*`). Nothing else does.

**Why.** Wire contract drift, storage migrations, and runtime feature work
are independent concerns. Coupling them creates cascading breakage.

**Enforced by.** Lint rule `B1-layer-direction`: import-graph check using
`golang.org/x/tools/go/packages`.

**Override.** Not allowed.

---

## B2. Observe vs do — Kave does not initiate

**What.** Kave receives calls from agents, forwards to providers, observes.
Kave never initiates an agent action, never schedules an LLM call, never
polls a downstream system on its own clock. The only outbound HTTP/gRPC
calls are:
- The LLM gateway forwarding a received request to a provider
  (`server/internal/gateway/transport.go`).
- Outbound connectors invoked synchronously inside the request lifecycle
  (`core/connectors/outbound/*`).
- Observability sinks (audit, OTel exporter) and FX rate sync (a single
  scheduled refresh — see exception below).

**Why.** This is the product's hard rule (see `product_vision.md`). The
moment Kave runs an agent on its own clock, it has crossed from middleware
into runtime, and the value proposition collapses.

**Enforced by.**
- Lint rule `B2-no-schedulers-in-core`: no imports of cron libraries
  (`robfig/cron`, `quartz`, etc.) in `core/`.
- Lint rule `B2-no-tickers-in-core`: `time.NewTicker`, `time.AfterFunc` in
  `core/` requires `// allow:B2 <reason>` plus an entry in the allowlist
  file.
- Lint rule `B2-egress-chokepoint`: HTTP `client.Do(...)` may only appear in
  `core/connectors/outbound/*` and `server/internal/gateway/transport.go`.
- Architectural test `TestKaveProducesNoOutboundAtIdle`: harness boots the
  server with a network observer, idles 5 seconds; zero outbound packets.
  (Exception: FX refresh, which is opted into via the allowlist.)

**Override.** Limited list:
- FX scheduled refresh (`core/fx`) — declared in lint allowlist with
  reason "scheduled rate sync, observability not action."
- Stream-hub heartbeats (`core/streamhub`) — keepalive only.
- Audit retention sweep — bookkeeping, not action.

Anything else proposing a ticker requires a written justification appended
to this file before the allowlist entry is accepted.

---

## B3. Budgets are agent-scoped (v1)

**What.** v1 has exactly one budget surface: `policy.CostPolicy.BudgetCap`
on a policy bound to an agent. Org / project / env / user budgets are
post-v1.

**Why.** The post-v1 "Team features" item is where multi-level budgets live
(per `product_vision.md`). Adding the levels now means the test matrix
explodes for no v1 customer benefit.

**Enforced by.**
- Code seam: a `Budgetable` interface in `core/runtime/budget` with one
  method `BudgetCap() *money.Amount`. v1 implementers: `Agent` only.
- Lint rule `B3-budget-cardinality`: any `BudgetCap` field added to
  `Project`, `Environment`, `Organization`, or `User` model fails CI.

**Override.** Removed only by an explicit post-v1 plan that updates this
file alongside the schema.

---

## B4. Identity ≠ authorization

**What.** Two layers, separately mockable:
- **Identity** (who): PASETO sessions, agent tokens, API tokens. Lives in
  `server/ops/auth`. Produces `authctx.Identity`.
- **Authorization** (what): casbin enforcement. Lives in `server/ops/policy`.
  Reads identity from ctx; emits allow / deny + reason.

**Why.** Mixing them produces handlers that are correct on the happy path
and wrong on every edge. Independent mocks let tests cover identity
failures and authz failures without touching each other.

**Enforced by.**
- Lint rule `B4-no-policy-in-auth`: `server/ops/auth/*` may not import
  `server/ops/policy/*`.
- Lint rule `B4-no-auth-in-policy`: `server/ops/policy/*` may not import
  `server/ops/auth/*` (it reads `authctx` only).

**Override.** Not allowed.

---

## B5. Frameworks parse; LLMs talk

**What.** Two distinct connector kinds:
- `core/connectors/inbound/frameworks/*` — parses incoming
  framework-shaped requests (LangChain, OpenAI Agents SDK, Claude Code).
  Produces a normalized `Action`. Never makes outbound calls.
- `core/connectors/outbound/llm/*` — talks to provider APIs. Receives a
  prepared request, returns a parsed response. Never parses
  framework-shaped traffic.

**Why.** These are different concerns. Mixing produces the kind of file
where the same package both decodes Claude Code SSE *and* speaks Anthropic
API — which makes the gateway nearly impossible to test.

**Enforced by.** Lint rule `B5-framework-llm-separation`: 
`core/connectors/inbound/frameworks/*` cannot import
`core/connectors/outbound/llm/*`. The gateway is the only place that holds
both.

**Override.** Not allowed.

---

## B6. Three stores, no cross-store transactions

**What.** `AppStore`, `SpanStore`, `AuditStore` are independent. `WithTx`
lives only on `AppStore`. Closures passed to `WithTx` may not call
`SpanStore` or `AuditStore` methods. Span writes and audit appends happen
*after* the AppStore commit.

Additionally: `SpanStore.OpenSpan` / `CloseSpan` are called only from
`core/runtime/trace/*` and tests (D9 — single span writer).

**Why.** Two-phase commit across heterogeneous stores is a swamp. Eventual
consistency between the control plane (must-be-correct) and observability
(can-be-best-effort) is intentional.

**Enforced by.**
- Lint rule `B6-no-cross-store-tx`: AST scan of all `WithTx` callers; if
  the closure references `SpanStore` or `AuditStore` symbols, fail.
- Lint rule `B6-single-span-writer`: `OpenSpan` / `CloseSpan` may only be
  referenced from `core/runtime/trace/*` and `*_test.go` files.

**Override.** Not allowed.

---

## B7. Money in int; float only in PriceBook

**What.** `core/pkg/money.Amount` is the in-process money type. Its zero
value is well-defined. Floats appear only in `server/ops/cost/service.go`
(the price book is config, not accounting).

**Why.** Float dollars are a famous accounting bug. PriceBook is a
configuration shape (cents-per-million-tokens) where double precision is
fine; once it leaves the price book and becomes a charge, it's int.

**Enforced by.** Lint rule `B7-no-float-money`: any `float32` / `float64`
declaration whose identifier matches `(?i)cost|price|amount|spend|budget`
outside `server/ops/cost/service.go` fails.

**Override.** Not allowed inside `core/`. In `server/` requires
`// allow:B7 <reason>` and an allowlist entry.

---

## B8. All IDs from `core/pkg/ids`

**What.** Every persistent ID is produced by `ids.New(prefix)` (or
`ids.TraceID()` / `ids.SpanID()` for OTel hex). Hand-rolled prefix
concatenation and `uuid.NewString()` are forbidden.

**Why.** Project invariant from `proto_architecture.md`. ULIDs are
lexicographically sortable; prefixes are debuggable. Mixed schemes produce
heisenbugs in cursor pagination.

**Enforced by.**
- Lint rule `B8-no-uuid`: no imports of `github.com/google/uuid` outside
  test files.
- Lint rule `B8-no-manual-prefix`: AST scan for `prefix + "_" + ulid` style
  expressions; fail outside `core/pkg/ids`.

**Override.** Not allowed.

---

## B9. Time as int64 unix-ms in models and proto

**What.** Storage shapes (`core/model/*`) and wire shapes (`proto/*`) use
`int64` unix-ms. `time.Time` appears only at edges (parsing, formatting,
business logic). Prefer `core/pkg/timex` for new business-logic timestamps.

**Why.** Cross-language wire compatibility, deterministic test fixtures,
no timezone surprises in storage.

**Enforced by.**
- Lint rule `B9-no-time-in-models`: AST scan of struct fields in
  `core/model/*` and `proto/gen/*`; fail on any `time.Time` field.

**Override.** None for persisted model fields.

---

## B10. HTTP route allowlist

**What.** After v1, the entire HTTP surface is:
- `GET /health`
- `POST /v1/openai/*`
- `POST /v1/anthropic/*`
- `POST /v1/google/*`
- `* /frameworks/<name>/*`

Everything else is gRPC over the listening socket (or `bufconn` in tests).
Adding a route requires updating `cmd/lint-architecture/allowlist/http.txt`
and this file.

**Why.** Plan 15 closed the HTTP bridge. Re-opening it by accretion is the
exact failure mode that caused the bridge incident in the first place.

**Enforced by.** Lint rule `B10-http-allowlist`: scan for `mux.HandleFunc`,
`http.HandleFunc`, `Router.Handle*` callers; route argument must match a
pattern in the allowlist. Unmatched route fails CI with file:line and
"add to `cmd/lint-architecture/allowlist/http.txt` if intentional."

**Override.** Edit `allowlist/http.txt` (one route per line, with reason).
The git diff on that file is the architecture review.

---

## How rules are added

1. Edit this file with `what / why / enforced by / override`.
2. Add the rule to `cmd/lint-architecture/rules/`.
3. Add a passing fixture and a failing fixture under
   `cmd/lint-architecture/testdata/`.
4. Bump `RulesVersion` in the linter so caches invalidate.
5. PR review: the rule file diff is the rule change.

## How rules are removed

Same process, in reverse, plus a one-paragraph note in `v1_decisions.md`
explaining why a v1-frozen rule was relaxed and what replaces it.
