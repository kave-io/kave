# Kave v1 Backlog — Plans

One plan = one haiku agent session. Ordered by dependency.

1. [01-grpc-completion.md](01-grpc-completion.md) — finish gRPC surface (missing RPCs, cursor pagination, control ops).
2. [02-http-bridge.md](02-http-bridge.md) — thin JSON-over-gRPC bridge + explicit streaming handlers.
3. [03-streaming-events.md](03-streaming-events.md) — typed event bus + JSONL streams (trace/span/events/logs).
4. [04-daemon-lifecycle.md](04-daemon-lifecycle.md) — status, doctor, reload, config-diff.
5. [05-config-layered.md](05-config-layered.md) — 5-layer kave.yaml merge + env expansion + watched reload.
6. [06-auth-rbac.md](06-auth-rbac.md) — real user/session/token APIs + casbin bindings + enable interceptor.
7. [07-gateway-proxy.md](07-gateway-proxy.md) — /v1/openai etc routes, connector lifecycle, enforce policy/budget in proxy.
8. [08-trace-model.md](08-trace-model.md) — trace_id tree, richer filters, export backends (mermaid/dot/jsonl/otlp/parquet).

### Workflow
User hands plan to a haiku agent. Returns. Opus reviews diffs; issues next two plans.

### Invariants every plan must preserve
- Use `core/pkg/ids.New(prefix)` for all IDs (never `uuid.NewString()`).
- All HTTP JSON responses go through `server/internal/contract.WriteSuccess` / `WriteError`.
- Money fields use `contract.Money{Amount, Currency}`. Paired time fields: `<name>` (ISO) + `<name>_ms`.
- Cursor pagination everywhere: `store.Page{Limit, Cursor}` in, `PageResult[T]{Items, NextCursor}` out.
- No breaking changes to proto wire format without updating `proto/` and regenerating.
