# Kave — Session Handoff

## What was built this session

### `core/` — complete and fully tested

All packages done, all tests passing (`go test ./...` clean):

| Package | What's in it |
|---|---|
| `intercept/` | `Unit`, `Action`, `Event`, `Run`, `Policy`, `Result`, `Interceptor` interface, `Pipeline`, context helpers |
| `auth/` | `PolicyEngine` interface, `AllowAll`/`DenyAll` test implementations |
| `trace/` | `Span` struct, `Tracer` interface, `SourceIntercept`/`SourceReport`/`SourceOTELImport` constants, `ComputeDuration()` |
| `cost/` | `BudgetStatus`, `Meter` interface, `NewBudgetStatus()` constructor |
| `validate/` | `Schema`, `Validator` interface |
| `pkg/money/` | `Amount` as `int64` nano-dollars, `FromDollars`, `Dollars()`, `String()` — 20 tests |
| `pkg/timex/` | `MS` named type for UnixMilli timestamps, zero-value-as-nil, `Now/From/Since` — 17 tests |
| `pkg/fp/` | functional helpers |
| `pkg/pointer/` | pointer utilities |

**Key design decisions (confirmed):**
- Money: `int64` nano-dollars (1 nano = 10^-9 USD), no float64 anywhere in monetary math
- Timestamps: `timex.MS` named type — zero value = "not set", no pointer needed for optional timestamps
- Three-way model: `Action` (causal, can block) / `Event` (reported, audit only) / `Span` (immutable record)
- `Unit` struct embedded by both `Action` and `Event`

---

### `connectors/` — LLM connectors complete, others stubs

**Implemented LLM connectors** (all build and vet clean):

| Connector | Package | Status | APIVersion |
|---|---|---|---|
| OpenAI | `llm/openai/` | `api.go` + `client.go` + `connector.go` + `doc.go` | `v1` |
| Anthropic | `llm/anthropic/` | `api.go` + `client.go` + `connector.go` + `doc.go` | `2023-06-01` |
| Gemini | `llm/gemini/` | `api.go` + `client.go` + `connector.go` + `doc.go` | `v1beta` |
| Ollama | `llm/ollama/` | full client + session + `connector.go` + `doc.go` | `0.6.x` |

**Connector pattern:**
- All connectors are middleware wrappers: validate `action.Connector`, call `next(ctx, action)`, return
- Clients (HTTP calls) live in `client.go`, used by `server/proxy` via `next`
- `Capabilities.APIVersion` string added to root `Capabilities` struct for runtime inspection
- Each package has `const APIVersion` for compile-time reference
- `doc.go` in each package: auth quirks, versioning notes, upgrade path

**Stub connectors (empty packages, compile fine):**
- `llm/groq/` — skipped per user decision
- `llm/gemini/`, `llm/anthropic/` — done
- `frameworks/`: langchain, crewai, openai-agents, openclaw, autogen — stubs
- `protocols/`: mcp, a2a, otel — stubs (deferred)
- `tools/`: stripe, github, postgres, s3, slack, gmail, zarinpal — stubs

---

## What's next: `server/`

The server module (`github.com/kave-io/kave/server`) is the next phase. It wires everything together.

### Already exists (from git status, untracked files):
```
server/main.go
server/proxy/proxy.go          ← HTTP proxy for LLM calls
server/proxy/proxy_test.go
server/proxy/upstream.go
server/pipeline.go
server/auth/
server/config/
server/cost/
server/db/
server/infra/                  ← postgres, casbin, paseto, riverq, crypto, pools
server/store/
server/trace/
server/go.mod / go.sum
```

### Build order for server/:
1. **`server/infra/`** — external service wrappers (postgres pool, casbin, paseto, crypto) — already exist, need review
2. **`server/db/`** — migrations (schema already in `001_kave_core.sql`), sqlc queries
3. **`server/store/`** — AppStore (SQLite), SpanStore (DuckDB) implementations
4. **`server/proxy/proxy.go`** — HTTP proxy intercepting LLM calls (exists, needs review)
5. **`server/api/`** — REST API handlers (not yet)
6. **`server/grpc/`** — gRPC endpoints (not yet)

### Proxy hot path (from design spec):
```
POST /openai/v1/chat/completions
→ extract provider from path
→ resolve tap + policy (O(1) cache)
→ resolve/create Run
→ create Action
→ AUTH CHECK (policy.allowed_types/connectors/methods)
→ BUDGET CHECK (if cap set)
→ inject credential (replace Authorization header)
→ forward to upstream (next handler)
→ parse token usage from response
→ update Action + Run
→ [ASYNC] write Span
→ return response
```

### Key infrastructure decisions (from CLAUDE.md):
- **AppStore**: SQLite (`~/.kave/kave.db`) — runs/actions/policies/taps
- **SpanStore**: DuckDB (`~/.kave/spans.db`) — immutable spans
- **Policy cache**: in-memory, write-through, 60s TTL safety net
- **Unix socket**: `~/.kave/kave.sock` for CLI↔daemon comms
- **Ports**: proxy `:4000`, dashboard `:4005`
- **IDs**: UUID v7 everywhere
- **Timestamps**: `int64` UnixMilli (maps to `timex.MS`)
- **Money**: `int64` nano-dollars (maps to `money.Amount`)

---

## Agent workflow preferences

- Use **haiku** for all subagents (opus is too expensive)
- Always **review haiku results yourself** before acting — haiku can produce wrong field names, bad analysis
- Design-first: spec → flows → architecture → code. Never code before alignment.
- One step at a time. Each phase reviewed before moving to next.

---

## Commands

```bash
# From monorepo root (/home/ali/Projects/kave)
cd core && go test ./...          # all core tests
cd connectors && go build ./...   # all connectors build
cd server && go build ./...       # server (check current state first)

# Hot reload
make dev

# DB
make migrate
make sqlcgen
```

## Repo layout

```
/home/ali/Projects/kave/
├── core/           github.com/kave-io/kave/core       ← DONE
├── connectors/     github.com/kave-io/kave/connectors ← LLM done, rest stubs
├── server/         github.com/kave-io/kave/server     ← NEXT
├── go.work         workspace file
└── CLAUDE.md       full architecture spec
```
