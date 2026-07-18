<div align="center">
  <img src="https://raw.githubusercontent.com/kave-io/docs/main/src/assets/kave-full.webp" alt="Kave" width="72" />
  <h1></h1>
  <p><strong>The control plane for AI agents.</strong></p>
  <p>Observe, authorize, validate, and cost-control agent actions across frameworks, models, and tools.</p>
</div>

## Status

Kave is pre-v1. The existing V1 control/runtime platform remains available
while the pre-release V2 kernel is developed beside it. V2 is currently a
Postgres-only, machine-to-machine admission, provider-gateway, secrets, usage,
and audit kernel with a Connect control API. Its wire and persistence contracts
may still change until V2 is released; upgrade the V2 server and SDK together.

## What Is Here

Kave provides:

- A compact V2 kernel for namespace isolation, scoped machine credentials,
  atomic limits, provider routing, usage, and audit evidence.
- Runtime tracing for runs, actions, spans, and costs.
- Control-plane APIs for agents, policies, credentials, budgets, RBAC, and audit logs.
- A framework/model gateway for LLM and tool traffic.
- A CLI and Vue dashboard for local operation.
- An architecture linter for the boundary decisions in `docs/design/`.

## Repo

```text
core/                  Go domain models, runtime contracts, connectors, mappers
server/                Go server, stores, gateway, auth, policy, budget, tracing
cli/                   Kave CLI
proto/gen/             Generated protobuf module
cmd/lint-architecture/ Architecture boundary linter
dashboard/             Vue dashboard
docs/                  Design notes and remaining plans
```

## Common Commands

```bash
make build
make test-fast
make fmt

cd server && go run .
cd dashboard && bun run dev
```

## Documentation

- V2 operator guide: `docs/v2-operations.md`
- Architecture decisions: `docs/design/`
- Remaining plans: `docs/plans/`
- External docs repo: https://github.com/kave-io/kave-docs

## License

Apache 2.0. See `LICENSE`.
