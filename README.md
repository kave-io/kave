<div align="center">
  <img src="docs/assets/kave-logo.svg" alt="Kave" width="72" />
  <h1>Kave</h1>
  <p><strong>The control plane for AI agents.</strong></p>
  <p>Observe, authorize, validate, and cost-control every agent action — across any framework, any model, any language.</p>

  <p>
    <a href="https://kave.io/docs"><img src="https://img.shields.io/badge/docs-kave.io-0F6E56?style=flat-square" /></a>
    <a href="https://github.com/kave-io/kave/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue?style=flat-square" /></a>
    <a href="https://github.com/kave-io/kave/releases"><img src="https://img.shields.io/github/v/release/kave-io/kave?style=flat-square&color=85B7EB" /></a>
    <a href="https://discord.gg/kave"><img src="https://img.shields.io/badge/discord-join-7289da?style=flat-square" /></a>
    <img src="https://img.shields.io/badge/built%20with-Go-00ADD8?style=flat-square&logo=go" />
  </p>
</div>

---

## What is Kave?

AI agents are moving into production. They call APIs, read databases, send emails, and make decisions — autonomously, at scale. Most teams have no idea what their agents are doing, what they cost, or what they can access.

**Kave sits between your agents and the world.** Every action passes through Kave's intercept layer, which gives you:

- **Observability** — full trace tree of every LLM call, tool use, and decision. Replayable, searchable, debuggable.
- **Auth & access control** — agent identities, scoped permissions per tool call, ephemeral credentials. Zero shared API keys.
- **Output validation** — schema enforcement, retry policies, content guardrails before outputs touch your systems.
- **Cost control** — token budget enforcement per agent/team, spend attribution, model routing recommendations.

Kave doesn't run your agents. It makes them observable, controllable, and affordable — across any framework, any model, any runtime.

## Quickstart

```bash
# self-host with docker compose
curl -sSL https://kave.io/install | bash

# or install the cli
go install github.com/kave-io/kave/cli/cmd/kave@latest
```

```bash
# initialize kave in your project
kave init

# watch your agent's actions live
kave watch --agent my-agent
```

```go
// wrap any LLM client — one line
import "github.com/kave-io/kave-go"

client := kave.Wrap(openai.NewClient(apiKey),
  kave.WithTracing(),
  kave.WithCostTracking(),
  kave.WithBudget("agent-1", 50.00), // $50/month hard cap
)
```

Or point your existing agent at Kave's proxy — no code changes:

```bash
export OPENAI_BASE_URL=https://your.kave.instance/proxy/openai
```

## The four modules

| Module | What it does |
|--------|-------------|
| `core/trace` | Records every agent action as a structured, replayable span tree |
| `core/auth` | Agent identities, scoped permissions, ephemeral credentials, audit logs |
| `core/validate` | Schema validation, retry policies, content guardrails on all outputs |
| `core/cost` | Token metering, budget enforcement, spend attribution, routing recommendations |

## Connectors

Kave works with everything — via native SDK wrappers, HTTP proxy, or OpenTelemetry:

**Frameworks** — LangChain · LangGraph · CrewAI · OpenAI Agents SDK · OpenClaw · AutoGen · Google ADK  
**Models** — OpenAI · Anthropic · Ollama · Gemini · Groq · Mistral · DeepSeek · any OpenAI-compatible endpoint  
**Tools** — GitHub · Stripe · Zarinpal · Gmail · Slack · PostgreSQL · S3 · and more  
**Protocols** — MCP (wraps any MCP server) · OpenTelemetry · A2A  

## Repo structure

```
kave/
├── core/          # the engine — 4 modules, pure Go, zero dependencies
├── connectors/    # pluggable adapters for frameworks, models, tools, protocols
├── server/        # deployable kave server (REST + gRPC + HTTP proxy)
├── sdk/           # go · python · typescript · openapi spec
├── cli/           # kave CLI — init, watch, trace, simulate, deploy
├── dashboard/     # nuxt 4 web dashboard
├── docs/          # mintlify documentation
└── deploy/        # docker-compose · helm · fly.io
```

## Draft concepts

- [Counterfactual Runs (shadow execute, replay, and drift branching)](IDEAS.md)

## Self-hosting

```bash
# one command, runs postgres + clickhouse + kave server
docker compose -f deploy/docker-compose.yml up -d
```

## Contributing

Kave is open source under the Apache 2.0 license. Connectors are the easiest place to contribute — each one is a small, isolated Go package that implements a single interface.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the guide.

## License

Apache 2.0 — see [LICENSE](LICENSE).

---

<div align="center">
  <sub>Built with ⬡ by <a href="https://github.com/kave-io">kave-io</a></sub>
</div>
