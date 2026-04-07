# Kave — Control Plane for AI Agents

## Architecture

**Kave is a middleware control plane for AI agents.** Think of it like Express/Fastify middleware,
but instead of handling HTTP requests from humans, it handles actions from AI agents:
LLM calls, tool uses, memory reads, API calls.

Every agent action passes through Kave's **intercept pipeline** which enforces four modules in order:

1. **trace** — record what happened, inputs, outputs, latency, model used
2. **auth** — is this agent allowed to do this action with these parameters?
3. **validate** — does the output match the expected schema?
4. **cost** — meter tokens, enforce budget, attribute spend

This is a **runtime-agnostic** control plane. It works with any agent framework:
LangChain, OpenAI Agents SDK, CrewAI, OpenClaw, plain API calls, etc.

---

## Core Data Model

### `Run`

One agent task from start to finish. Has budget, policy, status.

### `Action`

One thing an agent does within a run: an LLM call, tool use, memory read/write.

### `Span`

The trace record of one action: timing, input, output, cost, error.

### `Policy`

What a run/agent is allowed to do: permissions, budget cap, output schema.

---

## Monorepo Structure

The monorepo uses `go.work` with these Go modules:

```
core/               github.com/kave-io/kave/core
├── intercept/     — Action, Pipeline, Interceptor interface
├── trace/         — Tracer interface + Span model
├── auth/          — PolicyEngine interface + Token/Scope
├── cost/          — Meter interface + BudgetStatus
├── validate/      — Validator interface + Schema/Guardrail
└── pkg/           — shared utilities (constants, fp, pointer)

connectors/        github.com/kave-io/kave/connectors
├── connector.go   — Connector interface
├── llm/          — LLM providers (openai, anthropic, gemini, groq, etc.)
├── frameworks/   — agent frameworks (langchain, crewai, openai-agents, openclaw)
├── tools/        — tool connectors (stripe, github, databases, etc.)
└── protocols/    — protocol implementations

server/           github.com/kave-io/kave/server
├── infra/        — external services (postgres, redis, casbin, paseto, ollama, etc.)
├── api/          — REST API handlers
├── grpc/         — gRPC endpoints
├── proxy/        — HTTP proxy for LLM calls
├── config/       — configuration loading
├── db/           — migrations
└── main.go       — server entrypoint

cli/              github.com/kave-io/kave/cli
├── cmd/          — Cobra CLI commands
└── main.go       — CLI entrypoint

sdk/               Public SDKs (not Go modules)
├── go/            — Go SDK
├── http/          — HTTP client
├── typescript/    — TypeScript/Node SDK
└── python/        — Python SDK
```

### Dependency Rules (strict)

```
✅ core → nothing (pure Go, minimal deps)
✅ connectors → core
✅ server → core + connectors
✅ cli → core + connectors
✅ sdk → core (for type compatibility)

❌ core → connectors (never)
❌ core → server (never)
```

---

## Infrastructure (server/infra/)

**In use:**

- `postgres/` — PostgreSQL connection pool, migrations, transactions
- `casbin/` — RBAC policy engine for auth module
- `paseto/` — PASETO token generation (JWTs)
- `crypto/` — AES encryption for secrets
- `pools/` — connection pool manager
- `riverq/` — job queue for async tasks


## Core Packages (core/pkg/)

**In use:**

- `constants/` — app-wide constants
- `fp/` — functional programming helpers (Map, Filter, Reduce, etc.)
- `pointer/` — pointer utilities (Ptr, Deref)

## Database Schema

All tables in `server/db/migrations/001_kave_core.sql`:

- `workspaces` — multi-tenancy root (workspace_id on all other tables)
- `agents` — agent identities (policy_id, metadata)
- `policies` — permissions, budget cap, allowed connectors/methods
- `runs` — agent executions (status, spent_usd, error_message)
- `actions` — individual actions within a run (action_type, connector, method)
- `spans` — trace records (started_at, ended_at, tokens, cost_usd)
- `budget_ledger` — immutable append-only cost records

All tables use `UUID` primary keys, `TIMESTAMPTZ` for timestamps, and appropriate B-tree/GIN indexes.
Mutable tables have `updated_at` trigger. Budget ledger is append-only (no trigger).

---

## Interface Contracts

### `core/intercept/Interceptor`

```go
type Interceptor interface {
    Before(ctx context.Context, action *Action) (*Action, error)
    After(ctx context.Context, action *Action, result *Result) error
    Name() string
}
```

Every module (auth, validate, trace, cost) and every connector implements this.

### `core/intercept/Pipeline`

```go
type Pipeline struct { ... }
func (p *Pipeline) Execute(ctx, action, handler) (*Result, error)
```

Chains interceptors: calls all Before hooks, executes handler, calls all After hooks in reverse.

### `connectors/Connector`

```go
type Connector interface {
    Name() string
    Intercept(ctx context.Context, action *Action, next Handler) (*Result, error)
    Capabilities() Capabilities
}
```

Every connector (OpenAI, Stripe, database, framework) implements this.

---

## Local Development

### Prerequisites

- Go 1.26
- Docker + docker-compose (for Postgres, Ollama, etc.)
- PostgreSQL 18+
- Make

### Commands

```bash
# Hot reload Go with Air
make dev

# Run tests (uses testcontainers, pulls Docker image)
make test-fast

# Generate type-safe queries from SQL (uses sqlc)
make sqlcgen

# Run database migrations
make migrate

# Code quality
make lint
make fmt
make vet
```

### Database Setup

```bash
# Docker Compose (in deploy/)
docker-compose -f deploy/docker-compose.yml up -d

# Migrations auto-run on server startup, or:
make migrate
```

### Ollama (Local LLM)

```bash
# Download model
ollama pull mistral

# Start server (runs on localhost:11434)
ollama serve
```

---

## How to Add a Connector

Connectors are the "leaf nodes" that actually execute agent actions.

1. Create `connectors/<type>/<name>/connector.go`
2. Implement `connectors.Connector` interface:
   - `Name()` → unique ID like "openai", "stripe"
   - `Intercept()` → wrap execution with pipeline
   - `Capabilities()` → declare supported actions/methods
3. Create tests in `*_test.go`
4. Import in `server/` or `cli/` where needed

Example: `connectors/llm/openai/connector.go` is a template.

---

## How to Add an Interceptor Module

Interceptors enforce policy, trace, validate, and cost on every action.

1. Create `core/<module>/<interface>.go`
2. Define interface with `Before()`, `After()`, `Name()` methods
3. Server creates implementations that import from `server/infra/`
4. Add to pipeline in order: auth → validate → trace → cost

---

## Conventions

- **Go version:** 1.26, idiomatic Go, no magic
- **Errors:** returned, never panicked (except main startup)
- **Context:** always first parameter in signatures
- **Interfaces:** defined in package that uses them, not implements them
- **Tests:** table-driven where applicable, use testcontainers for DB
- **Commits:** conventional commits (feat:, fix:, chore:, docs:)
- **No global state:** everything injected via constructors
- **Config:** YAML files with type-safe structs, sensible defaults, validation

---

## Key Files

- `core/intercept/interceptor.go` — Action, Result, Interceptor
- `core/intercept/pipeline.go` — Pipeline execution logic
- `core/trace/tracer.go` — Tracer interface
- `core/auth/policy.go` — PolicyEngine interface
- `core/cost/meter.go` — Meter interface
- `core/validate/schema.go` — Validator interface
- `connectors/connector.go` — Connector interface template
- `server/db/migrations/001_kave_core.sql` — core schema
- `server/infra/postgres/postgres.go` — database setup
- `server/infra/casbin/casbin.go` — policy engine
- `server/infra/paseto/paseto.go` — token generation

---

## Current State (as of 2026-04-06)

### Done
- `core/` — full intercept pipeline: Action, Result, Pipeline, Interceptor, Run, Policy, context helpers
- `connectors/` — interfaces + stubs for all LLM providers (OpenAI, Anthropic, Gemini, Groq, Ollama), frameworks (LangChain, CrewAI, OpenAI Agents, AutoGen, OpenClaw), tools (GitHub, Stripe, Slack, Gmail, Postgres, S3, Zarinpal), protocols (MCP, OTel, A2A)
- `server/infra/` — Postgres, Casbin (RBAC), PASETO tokens, AES crypto, connection pools, RiverQ job queue
- `server/auth/` — CasbinPolicyEngine wired to core auth interface
- `server/cost/` — PostgresMeter + pricing tables
- `server/trace/` — PostgresTracer wired to core trace interface
- `server/store/` — app store (SQLite + Postgres) + span store (DuckDB + Postgres) with factory
- `server/proxy/` — HTTP proxy skeleton (proxy.go + upstream.go)
- `server/pipeline.go` — server-side pipeline assembly
- `server/config/` — YAML config with type-safe structs
- `server/db/` — migration runners for SQLite and DuckDB
- `cli/cmd/` — root, start, stop, status, socket, trace commands

### v1 Build Remaining

**server/api/ — REST API handlers (highest priority, nothing exists yet)**
- `POST /agents` — register agent (name, policy_id, metadata/persona)
- `GET /agents/:id` — get agent details
- `GET /runs` — list runs with filters
- `GET /runs/:id/spans` — full trace tree for a run
- `GET /spans` — span search/explorer
- `GET /cost/summary` — spend breakdown by agent/model/period
- `POST /policies` — create policy (budget cap, permissions, guardrails)
- `GET /health` — liveness probe

**server/proxy/ — Complete the LLM proxy**
- Route by provider prefix (`/proxy/openai`, `/proxy/anthropic`, etc.)
- Strip/inject auth headers (zero shared API keys)
- Run intercept pipeline on every proxied request
- Record span to store

**CLI — finish operational commands**
- `kave watch --agent <id>` — live tail spans via server socket
- `kave trace <run-id>` — print span tree for a run

**Agent persona — what's in scope for v1**
- Name, tag, description
- System-level policy prompt injection ("never return PII")
- `policies` tab: attach guardrail rules
- `budget` tab: token limit per agent
- `permissions` tab: allowed connectors/methods
- **Not in scope**: any flow builder, tool chaining, or agent workflow editor

### Post-v1 Roadmap (do not build now)

In priority order based on product strategy:

1. **Team/workspace features** — invite by email, role seats (admin/dev/viewer/billing), per-team budgets, activity feed. This is what converts free → paid.
2. **Cost intelligence** — monthly forecasting, model swap recommendations ("switch X to Sonnet, save 40%"), per-client attribution for freelancer invoicing.
3. **Replay & simulation sandbox** — replay any trace with different model/guardrails/prompt. Makes Kave part of the dev loop. Very sticky.
4. **Alerts & anomaly detection** — cost spikes, latency degradation, guardrail violations. Configurable thresholds, webhook+email+Slack delivery.
5. **Audit log & compliance export** — every action/policy change exportable as CSV/JSON. Required for SOC 2 and enterprise deals.
6. **Connector marketplace** — browse/enable community connectors from dashboard. Scales without building everything in-house.
7. **Dashboard/UI** — Nuxt 4 web app with trace explorer, cost breakdown, agent registry fleet view, agent persona pages.

**The line we don't cross**: Kave never gives users a canvas to define agent behavior, tools, or workflows. No flow builders, no drag-and-drop steps. Agent registration (name, policy, persona prompt) is governance metadata, not runtime definition. If a feature only works with a Kave-native runtime, it's out of scope.

---

## Questions?

Refer to memory in `/home/ali/.claude/projects/-home-ali-Projects-kave/memory/` for project philosophy and development principles.
