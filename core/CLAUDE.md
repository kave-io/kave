# Kave Core — Design Spec

## Product context

Kave is a control plane for AI agents. It sits between any agent/app and the outside world,
intercepting every action before it happens and recording everything after.

CLI-first. Like Docker. The server is embedded in the CLI process. Dashboard is optional.
Everything the dashboard does, the CLI can do.

First boot creates a default project, default tap, default policy. Developer sets one env var
and sees their calls immediately. That is the product.

---

## Vocabulary

| Term                 | Definition                                                                                                                                        |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Project**    | Top-level workspace. Everything belongs to a project.                                                                                             |
| **Tap**        | A configured interception point. Represents one connected app or service. Proxy type: point your LLM client's base URL at it.                     |
| **Persona**    | Optional named agent identity. Has a purpose, a system prompt, and a policy. Kave works without one — unidentified calls use the default policy. |
| **Policy**     | A control-policy composition root with typed sub-policies for auth, cost, trace, and validation.                                                  |
| **Run**        | A single execution from trigger to completion. A DAG of actions. Could be one LLM call or hundreds of nested agents.                              |
| **Action**     | One unit of work inside a run. Has a type, connector, method, input, output, and a parent (for DAG structure).                                    |
| **Span**       | Immutable analytics record of one action. Timing, tokens, cost, error. Goes to a separate store.                                                  |
| **Credential** | An encrypted API key stored by Kave. Injected into proxied requests at enforcement time. Agents never see the real key.                             |

---

## Action types

Five categories. Policies, auth rules, and budget enforcement differ by type.

| Type          | Meaning                                                        | Budget impact     |
| ------------- | -------------------------------------------------------------- | ----------------- |
| `llm`       | LLM call: chat, embed, image, audio                            | Yes — token cost |
| `tool`      | Tool or function invocation: MCP, function call, custom tool   | No                |
| `retrieval` | Data read: vector DB, SQL query, file read, memory, web search | No                |
| `mutation`  | Data write: DB write, file write, memory write                 | No                |
| `api`       | Outbound HTTP to external service: Stripe, GitHub, etc.        | No                |

---

## Proxy tap URL structure

Provider identified by path. Zero config for the developer.

```
http://localhost:4000/openai      → forwards to api.openai.com
http://localhost:4000/anthropic   → forwards to api.anthropic.com
http://localhost:4000/ollama      → forwards to localhost:11434
```

Developer sets one env var. Done.

---

## Data models

All IDs: UUID v7 (time-ordered, sortable, good B-tree performance).
All timestamps: `int64` UnixMilli (serialization-agnostic, no timezone issues).

### Project

```
id           string
name         string
slug         string      ← URL-safe, used in CLI flags
created_at   int64
```

### Tap

```
id           string
project_id   string
name         string
type         string      ← "proxy" | "sdk" | "sidecar"
status       string      ← "active" | "paused"
port         *int        ← proxy type only
targets      map[string]string  ← {"openai": "https://api.openai.com"}
persona_id   *string     ← default persona for calls on this tap (nil = use default policy)
created_at   int64
```

### Persona

```
id              string
project_id      string
name            string
description     string
system_prompt   *string
policy_id       string
created_at      int64
updated_at      int64
```

### Policy

One flat struct. Cached in memory. Never hit on hot path.

```
id           string
project_id   string
name         string

// auth
allowed_types       []string    ← ["*"] or ["llm","tool"]
allowed_connectors  []string    ← ["*"] or ["openai","stripe"]
allowed_methods     []string    ← ["*"] or ["chat.completions"]

// budget
budget_cap_usd    *float64      ← nil = unlimited
budget_period     string        ← "run" | "daily" | "monthly"
budget_behavior   string        ← "block" | "warn"

// trace
trace_input       bool
trace_output      bool
retention_days    int

// validation (v2 placeholder)
validation        map[string]any

created_at   int64
updated_at   int64
```

Default policy: `allowed_* = ["*"]`, no cap, trace everything, 30 day retention. Cannot be deleted.

### Run

```
id           string
project_id   string
tap_id       string
persona_id   *string     ← nil = persona-less run
policy_id    string      ← locked at run start, does not change mid-run
status       string      ← "active" | "completed" | "failed"
started_at   int64
ended_at     *int64
spent_usd    float64     ← running total, incremented on each llm action
error        *string
metadata     map[string]any
```

### Action

```
id           string
run_id       string
parent_id    *string     ← nil = root action of run
type         string      ← "llm" | "tool" | "retrieval" | "mutation" | "api"
connector    string      ← "openai" | "postgres" | "stripe"
method       string      ← "chat.completions" | "query" | "charges.create"
status       string      ← "pending" | "running" | "completed" | "failed" | "blocked"
input        []byte      ← raw JSON, captured from request
output       []byte      ← raw JSON, filled on completion
error        *string
started_at   int64
ended_at     *int64
depth        int         ← 0 = root, enables flat DAG scan without recursion
seq          int         ← sibling order within parent, enables ordered rendering
```

`depth` + `seq` together allow full DAG reconstruction and rendering in a single flat query.

### Span

Immutable. Written once. Goes to a separate span store (swappable to ClickHouse at scale).

```
id              string
run_id          string
action_id       string
connector       string
model           *string     ← "gpt-4o", nil for non-llm
started_at      int64
ended_at        int64
duration_ms     int64
input_tokens    *int
output_tokens   *int
cost_usd        *float64
error           *string
tags            map[string]string
```

### Credential

```
id           string
project_id   string
provider     string      ← "openai" | "anthropic"
label        string
encrypted    []byte      ← AES-256-GCM
key_hash     string      ← SHA256, for dedup
last_used_at *int64
created_at   int64
```

---

## Policy enforcement: cache strategy

Policy is read on every action — hottest read path. Dataset is tiny (5-50 policies per project).

- On `kave start`: load all policies into memory
- Hot path: O(1) memory lookup, zero DB hit
- Policy update: write DB first, update cache entry (write-through)
- TTL: 60s as safety net

Policy is resolved and locked into `Run.policy_id` at run start.
Policy changes apply to new runs only — not mid-run. Predictable behavior.

Joins exist in the schema but only happen at:

- Dashboard queries (acceptable — read-heavy, not latency-critical)
- Run start (once per run, acceptable)
- Never on per-action enforcement

---

## Confirmed decisions

| #  | Decision                                                                                                                                                                                                       |
| -- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1  | Action input/output stored as raw `[]byte` JSON — parse only when queried                                                                                                                                   |
| 2  | Policy: one control-policy root with typed sub-policies, cached in memory, write-through, TTL 60s                                                                                                         |
| 3  | Action and Span are separate — Action is mutable lifecycle, Span is immutable analytics                                                                                                                       |
| 4  | All IDs: UUID v7                                                                                                                                                                                               |
| 5  | All timestamps:`int64` UnixMilli                                                                                                                                                                             |
| 6  | Tap proxy URL: provider identified by path (`/openai`, `/anthropic`, `/ollama`)                                                                                                                          |
| 7  | Run locks policy_id at start — mid-run policy changes don't apply                                                                                                                                             |
| 8  | `depth` + `seq` on Action for efficient DAG rendering without recursive queries                                                                                                                            |
| 9  | Kave works without a persona — unidentified calls use the default policy                                                                                                                                      |
| 10 | Default project + tap + policy created at first `kave start`                                                                                                                                                 |
| 11 | Run boundary: default = one Run per request (always correct, connection pooling breaks TCP-based grouping); explicit `X-Kave-Run-Id` header attaches to an existing Run; `X-Kave-Parent-Id` builds the DAG |
| 12 | Process detection via `/proc` deferred to v2 — latency on hot path, Linux-only, breaks in containers                                                                                                        |
| 13 | Daemon ↔ CLI: Unix domain socket at `~/.kave/kave.sock` — no port conflicts, no TIME_WAIT buildup, file-system ACL                                                                                         |
| 14 | Three-way model: `Action` (Kave causal, can block), `ObservedAction` (agent-reported, audit only, no "blocked" status), `Span` (immutable record for all patterns). `Invocation` is the shared embedded base for Action and ObservedAction, split into `InvocationRef`, `InvocationTarget`, `InvocationData`, and `InvocationTiming`. `Action.Outcome` carries structured decision detail. |

---

## Package Architecture (V1 Rule)

Four-package pattern with clear boundaries:

| Package | Owns | Examples |
|---------|------|----------|
| `runtime/` | Live execution domain | Action, Run, Policy, Outcome, TokenUsage |
| `model/` | Persisted records (storage contracts) | ActionRecord, RunRecord, SpanRow, BudgetEntry |
| `ops/` | Module interfaces and decision types | auth.Decision, cost.BudgetStatus, validate.Result |
| `mappers/` | Translation layer (runtime ↔ model only) | ActionToRecord, RunToRecord, AuthDecisionToOutcome |

**Key principle:** If you need to convert between runtime and model, do it in mappers/. Don't scatter conversions. `model/` is storage-shaped and boring — no logic, just fields. `runtime/` is execution-ergonomic. `ops/` has module contracts, not durable entities.

**Naming convention:** Persisted model types use *Record suffix when they correspond to a runtime type:
- `runtime.Run` (live execution state) → `model/runtime.RunRecord` (persisted database row)
- `runtime.Action` (in-flight action) → `model/runtime.ActionRecord` (persisted action audit)
- Other persisted types that don't have a live runtime counterpart stay simple (SpanRow, BudgetEntry, PriceBook).

---

## Flows

### Flow 1 — First boot (`kave start`)

```
kave start
│
├─ 1. parse config (config.yaml or defaults)
├─ 2. check ~/.kave/ exists?
│       no  → create ~/.kave/
│             create kave.db  (SQLite — AppStore)
│             create spans.db (DuckDB — SpanStore)
│             run migrations
│             seed:
│               project  "default"
│               policy   "default"  (allow-all, 30d retention)
│               tap      "default"  (proxy, port 4000)
│             write ~/.kave/kave.pid
│       yes → open existing DBs
│             run any pending migrations
│
├─ 3. warm policy cache  (load all policies into memory)
├─ 4. start proxy server    → :4000  (handles LLM traffic)
├─ 5. start API server      → ~/.kave/kave.sock  (handles CLI commands)
├─ 6. start dashboard       → :4005  (optional, can be disabled)
│
├─ 7. print to stdout:
│       Kave is running
│       Proxy:     http://localhost:4000
│       Dashboard: http://localhost:4005
│       Socket:    ~/.kave/kave.sock
│
└─ 8. detach → background process
               write PID to ~/.kave/kave.pid
               stdout → ~/.kave/kave.log

CLI is now live. kave status, kave watch, etc. connect via ~/.kave/kave.sock.
```

---

### Flow 2 — Proxy enforcement (hot path)

Every LLM call through the proxy runs this path. Sync = blocks the response. Async = fire and forget.

```
client
  │
  │  POST http://localhost:4000/openai/v1/chat/completions
  │  Authorization: Bearer <client-key-or-anything>
  │  [X-Kave-Run-Id: <uuid>]      ← optional
  │  [X-Kave-Parent-Id: <uuid>]   ← optional
  ▼
proxy
  │
  ├─ [SYNC] extract provider from path  → "openai"
  ├─ [SYNC] look up tap config          → from memory
  ├─ [SYNC] resolve persona             → from X-Kave-Persona header, or tap.persona_id, or nil
  ├─ [SYNC] resolve policy              → persona.policy_id or default — O(1) cache hit
  │
  ├─ [SYNC] resolve Run
  │           X-Kave-Run-Id present?
  │             yes → load Run from AppStore (or create if not found)
  │             no  → create new Run  (id=uuid7, policy_id=resolved, started_at=now)
  │
  ├─ [SYNC] create Action
  │           id=uuid7, run_id, parent_id=X-Kave-Parent-Id,
  │           type=llm, connector=openai, method=chat.completions,
  │           status=running, input=request body, depth, seq
  │
  ├─ [SYNC] AUTH CHECK
  │           policy.auth.allowed_types contains "llm"?
  │           policy.auth.allowed_connectors contains "openai"?
  │           policy.auth.allowed_methods contains "chat.completions"?
  │             fail → action.status = blocked
  │                    return 403 to client   ← request dies here
  │             pass → continue
  │
  ├─ [SYNC] BUDGET CHECK  (only if policy.cost.budget_cap != nil)
  │           run.spent_usd >= policy.cost.budget_cap?
  │             block  → return 429 to client
  │             warn   → continue, attach warning header to response
  │
  ├─ [SYNC] inject credential
  │           look up Credential for project+provider
  │           replace Authorization header with stored key
  │           client key is discarded — never forwarded
  │
  ├─ [SYNC] forward request → https://api.openai.com/v1/chat/completions
  │           (streaming supported — proxy streams response back as it arrives)
  │
  ├─ [SYNC] receive response
  │           parse token usage from response body
  │           (input_tokens, output_tokens, model)
  │
  ├─ [SYNC] update Action
  │           status=completed, output=response body, ended_at=now
  │
  ├─ [SYNC] update Run
  │           spent_usd += cost  (atomic increment)
  │
  ├─ [ASYNC] open Span  ← non-blocking, buffered channel → SpanStore
  │           later close/finalize with end data
  │
  └─ return response to client
```

Everything above the async span write is synchronous and adds latency. Target: < 1ms overhead excluding network.

---

### Flow 3 — Run lifecycle

**Simple case (default, no headers):**

```
request arrives → Run created (auto) → Action created → auth → forward → respond → Run completed
     |___________________________________________________________|
                    one HTTP round trip = one Run
```

**Multi-action case (SDK wrapper or framework connector):**

```
agent wakes up
  │
  ├─ SDK creates Run  (POST /runs or auto on first call with X-Kave-Run-Id)
  │    Run.status = active
  │
  ├─ call 1: X-Kave-Run-Id: abc  →  Action 1  (depth=0, seq=0)
  │    └─ spawns tool call
  │         call 2: X-Kave-Run-Id: abc, X-Kave-Parent-Id: action1  →  Action 2  (depth=1, seq=0)
  │         call 3: X-Kave-Run-Id: abc, X-Kave-Parent-Id: action1  →  Action 3  (depth=1, seq=1)
  │
  ├─ call 4: X-Kave-Run-Id: abc  →  Action 4  (depth=0, seq=1)
  │
  └─ agent signals done  (DELETE /runs/abc  or  final call with X-Kave-Run-End: true)
       Run.status = completed
       Run.ended_at = now

DAG in memory:
  Run abc
  ├─ Action 1  (llm)
  │   ├─ Action 2  (tool)
  │   └─ Action 3  (retrieval)
  └─ Action 4  (llm)
```

`kave trace abc` renders this tree live as actions arrive.

---

## What's next

1. ~~Package architecture~~ → see `docs/architecture.md`
2. ~~Connector compatibility~~ → see `docs/connectors.md`
3. **Build** — top-down, one package at a time:
   `runtime/` → `policy/` → `pipeline/` → `ops/auth/` → `ops/trace/` → `ops/cost/` → `ops/validate/`

---

## Workflow

**How we work:**

- Design first, always. Spec → flows → architecture → code. Never code before alignment.
- One step at a time. Finish a phase, document it here, then move.
- Every decision gets recorded in the Confirmed Decisions table above with a number.
- When a decision is revisited, update the table — don't append contradictions.
- Code is written only after the design for that package is agreed on.
- Each package is reviewed before moving to the next.

**Code style:**

- Concise names — functions, interfaces, files, variables.
- Pure functions that do one thing.
- No comments unless the logic cannot be understood without one.
- Pointer-aware — know when you're heap-allocating and why.
- Complex data structures and algorithms only where they make a measurable difference.
- No speculative abstractions. Build what's needed now.

**Decision log format:**
Any decision made in conversation gets added to the Confirmed Decisions table immediately.
Format: sequential number, one-line description of what was decided.
