# Connector Compatibility Analysis

The question: does our current Action/Pipeline architecture support every connector type
we're planning — without getting blocked later?

Short answer: yes, with two constraints to note now.

Core owns the connector contracts in `ports/connectors.go`. The concrete adapters under
`connectors/` should implement those interfaces, not define the source of truth for them.

---

## The 4 integration patterns

Every connector Kave will ever support falls into one of these four patterns.
The Action model must absorb all of them.

---

### Pattern 1 — HTTP Proxy (Kave sits in the request path)

```
client → Kave proxy → provider API → Kave proxy → client
```

Kave is a transparent HTTP proxy. Blocks while forwarding. Synchronous.

**Who uses this:** OpenAI, Anthropic, Groq, Mistral, DeepSeek, Gemini, Ollama HTTP,
any OpenAI-compatible endpoint, OpenAI Agents SDK.

**What Kave does:**
- Receives raw HTTP request
- Extracts provider from path (/openai, /anthropic)
- Creates Action(type=llm, connector=openai, method=chat.completions, input=request body)
- Runs pipeline (auth → budget)
- Injects credential (replaces Authorization header)
- Forwards to provider
- Receives response, extracts token usage
- Fills Action.output, writes Span
- Returns response to client

**Action model fit:** perfect. HTTP req/resp serializes cleanly to []byte JSON.

**Connector lives in:** the framework-routed server gateway — provider logic lives in `connectors/llm/<provider>/`, while the server owns transport, policy, and credential orchestration.
Provider-specific logic (token extraction, pricing) lives in `connectors/llm/<provider>/`.

---

### Pattern 2 — SDK Report-In (external SDK sends events TO Kave)

```
agent framework → Kave SDK callback → Kave API server
                  (Python / JS / Go)
```

Kave does NOT sit in the request path. The agent's SDK hooks into framework events
and pushes them to Kave's API. Asynchronous. Non-blocking for the agent.

**Who uses this:** LangChain (CallbackHandler), CrewAI (hooks), OpenClaw (middleware),
Pydantic AI (middleware), Semantic Kernel (filter), AutoGen (OTel exporter).

**What the SDK does:**
- on_llm_start  → POST /api/actions  { type: llm, status: running, input: ... }
- on_llm_end    → PATCH /api/actions/:id  { status: completed, output: ..., tokens: ... }
- on_tool_start → POST /api/actions  { type: tool, parent_id: ..., status: running }
- on_tool_end   → PATCH /api/actions/:id  { status: completed, output: ... }

**What Kave does:**
- Receives the observed action, creates/updates Action record
- Runs pipeline (auth, trace, cost) against the action data
- Cannot block the agent (the call already happened)
- Auth in this mode = audit + alert, not block-before-execute

**Action model fit:** perfect. Events map directly to Action fields.

**Important distinction:** framework connectors (Python/JS callbacks) are NOT Go code
in `connectors/`. They live in:
- `sdk/python/kave/langchain.py`
- `sdk/python/kave/crewai.py`
- `sdk/typescript/kave/langchain.ts`

The Go side (`connectors/frameworks/`) contains the API handler that receives
their events and runs them through the pipeline.

**Auth implication:** in proxy mode, auth happens BEFORE the action executes (block).
In SDK report-in mode, auth happens AFTER (the call already fired). This is a fundamental
difference to document clearly in the auth module.

---

### Pattern 3 — Protocol Bridge (Kave wraps a protocol layer)

```
agent → Kave MCP proxy → MCP server (any tool)
```

Kave speaks the protocol natively and acts as a passthrough with enforcement.
Like pattern 1 but for non-HTTP protocols.

**Who uses this:** MCP (highest leverage — wraps hundreds of tools at once), A2A (v3).

**MCP specifically:**
- Agent points its MCP client at Kave's MCP server address
- Kave receives JSON-RPC: tools/list, tools/call
- On tools/call: creates Action(type=tool, connector=mcp:<server>, method=<tool_name>)
- Runs pipeline (auth enforced BEFORE forwarding — same as proxy pattern)
- Forwards to actual MCP server
- Returns result

**Why MCP is the multiplier:** one MCP bridge implementation covers every MCP-compatible
tool automatically. Any MCP server registered in Kave gets full auth + trace + cost
enforcement without a dedicated connector per tool.

**Action model fit:** perfect. MCP tool args → JSON → Action.input.

**Connector lives in:** `connectors/protocols/mcp/`

---

### Pattern 4 — Trace Import (Kave receives pre-formed traces)

```
agent (already OTel-instrumented) → OTLP exporter → Kave OTel endpoint → SpanStore
```

Kave is an OTel-compatible collector. Receives spans in OTLP format.
Enriches them with cost data and policy violations. Stores in SpanStore.

**Who uses this:** AutoGen/AG2 (OTel exporter), AWS Bedrock (CloudWatch OTel),
any agent already using OTel instrumentation.

**Critical difference from patterns 1–3:**
This is NOT an enforcement path. The calls have already happened.
Kave cannot block or enforce policy before execution.
This is purely observability + post-hoc analysis.

**What Kave does:**
- Receives OTLP spans (gRPC or HTTP)
- Maps OTel span attributes to Kave's Span model
- Enriches: computes canonical cost from token attributes + pricing table (no float contracts)
- Writes to SpanStore
- Does NOT create Action records (no lifecycle to manage)
- Surfaces in dashboard alongside intercepted runs

**Implication:** Kave needs a separate ingestion endpoint for OTel, distinct from the
pipeline. The SpanStore handles both intercepted spans and imported spans.
Add `kind` and `source` fields to Span for classification: action/observed_action/import and enforcement/report/otel_import.

**Connector lives in:** `connectors/protocols/otel/` (the OTel receiver/collector)

---

## Architecture compatibility matrix

| Pattern | Action created | Pipeline runs | Auth blocks | Span written |
|---------|---------------|---------------|-------------|-------------|
| 1 — HTTP proxy | yes | yes, sync | yes, before exec | yes, after |
| 2 — SDK report-in | yes | yes, async | audit only | yes, after |
| 3 — Protocol bridge | yes | yes, sync | yes, before exec | yes, after |
| 4 — OTel import | no | no | no | yes, direct |

The Action/Pipeline model handles patterns 1–3 cleanly.
Pattern 4 bypasses the pipeline and writes to SpanStore directly.

---

## Two constraints to carry forward

### Constraint 1 — Streaming SSE (Pattern 1)

LLM proxy responses can be streamed (`stream: true`). The proxy must:
- Stream chunks back to the client in real-time — cannot buffer the whole response
- Accumulate content and token usage from chunks
- Write Action.output and Span only when the stream closes

The Action model is fine (output gets written at completion).
The proxy implementation needs a streaming variant of the enforcement flow.
Note this in the proxy flow — do not solve now.

### Constraint 2 — Auth mode differs by pattern

Pattern 1 and 3: **preventive** — auth runs before the call, can block it.
Pattern 2: **detective** — auth runs after the call, can alert/flag but cannot block.

The auth module must know which mode it's in. Add `ExecutionMode` to context:
`"sync"` (patterns 1, 3) vs `"async"` (pattern 2).
Auth interceptor behavior differs based on this.

---

## Connector package structure (proposed)

```
connectors/
  protocols/
    proxy/          ← HTTP proxy engine (shared by all LLM providers)
    mcp/            ← MCP protocol bridge
    otel/           ← OTel collector/receiver (import path)
  llm/
    openai/         ← token extraction, pricing table, response parsing
    anthropic/      ← 4-token-type extraction, pricing
    ollama/         ← native Go SDK wrap
    generic/        ← any OpenAI-compatible endpoint
  frameworks/
    langchain/      ← API handler for LangChain SDK observed actions
    crewai/         ← API handler for CrewAI SDK observed actions
    openclaw/       ← middleware plugin + observed-action handler
  tools/
    stripe/         ← HTTP REST (wraps pattern 1 for tools)
    github/
    postgres/       ← pgwire protocol (pattern 3 for SQL)
    mcp_hosted/     ← convenience: pre-registered public MCP servers
```

`sdk/` contains the Python/JS/Go client libraries that implement pattern 2 callbacks.
`connectors/` contains the server-side handlers and protocol bridges (patterns 1, 3, 4).

---

## Verdict

No architectural blockers. The Action/Pipeline model absorbs all four patterns.
The two constraints (streaming, auth mode) are design notes, not blockers.

Next: package architecture for `core/`.
