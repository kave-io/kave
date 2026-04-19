Here is the full server-side backlog to make CLI v1 actually completable.

1. Contract foundation (applies to all APIs)

- Implement unified JSON envelope/error contract from docs/02-json-schemas.md (schema_version, kind, data, page, warnings, stable error code).
- Add cursor pagination everywhere (current list endpoints mostly ignore cursor/next token).
- Standardize money/time fields to spec ({amount,currency} and paired ISO + _ms).
- Switch IDs to prefixed ULID format (current code generates UUIDs in app layer).
- Add schema-version negotiation hooks required by spec.

2. gRPC completion

- Implement missing Runtime RPCs: CancelRun, GetPriceBook, GetSpendReport (currently unimplemented in app/runtime/runtime.go).
- Implement proper ListOrganizations (currently hardcoded empty in app/control/control.go).
- Implement real pagination tokens on all List* gRPC methods.
- Add missing control-plane operations required by CLI but absent today: policy delete/export/validate/test, agent restore/export, credential delete/test, connector ops, auth/
RBAC ops, budget ops, apply/diff ops.
- Decide whether these go into existing services or new gRPC services.

3. HTTP API completion for CLI coverage

- Current HTTP API is limited to agents/policies/runs/spans/cost/pricing/fx only.
- Add endpoints for: status, doctor, logs, watch feed, trace (list/get/tail/graph/export), span (get/tail/export), events (list/tail), full agent/policy/credential lifecycle,
budget, price import/export/refresh, connector lifecycle, auth, rbac, config (view/diff/reload/get/set/path), admin store.
- Add apply engine endpoint(s) for apply/diff with dependency-order plan and prune/wait behavior.

4. SSE/streaming/eventing

- Current SSE is only /api/v1/spans/stream and backed by one run-event type.
- Add dedicated streams for trace tail, span tail, events tail, daemon logs.
- Implement streaming contract: JSONL envelope lines, heartbeat frames, final StreamClosed.
- Expand event bus model beyond RunEvent to typed events (policy.*, budget.*, credential.*, agent.*, daemon.*) as required by spec.

5. Daemon behavior and lifecycle

- Add daemon status/introspection surface: pid, uptime, store health, connector health, price/fx freshness, proxy readiness, schema versions.
- Add doctor check runner on server side (daemon, stores, connectors, price/fx staleness, credential references, config merge validity).
- Add runtime reload flow: force reload endpoint + SIGHUP behavior + validation + “requires restart” detection.
- Implement config-diff against live daemon state.

6. Config system alignment (docs/03-kave-yaml-config.md)

- Server currently reads one Viper config model, not the 5-layer kave.yaml contract.
- Implement layered merge: builtin/system/user/project/env.
- Implement list-replace and resource-by-name overwrite semantics.
- Implement ${VAR}/${VAR:-default} expansion and direct env override map.
- Implement watched-file reload with safe apply ordering and rollback-on-invalid behavior.

7. Auth/RBAC hardening

- Wire real user auth/session/token APIs (kave auth *) and storage.
- Wire RBAC role/binding APIs (kave rbac *) and enforcement.
- Replace placeholder gateway auth behavior (currently bearer UUID shortcut).
- Re-enable auth interceptor in pipeline (main currently runs cost+trace only).

8. Gateway/proxy completeness

- Expose proxy routes expected by status exports (/v1/openai, /v1/anthropic, /v1/ollama etc), not only /frameworks/claude-code/.
- Add connector enable/disable/test runtime behavior and health probes.
- Ensure policy, budget, validation, trace capture are consistently enforced in proxy path.

9. Trace model completeness

- Implement true trace/session grouping (trace_id, parent/child tree), not just run-level flat spans.
- Implement trace graph rendering/export backends (mermaid, dot, json, jsonl|otlp|parquet).
- Add richer trace/span filters required by CLI docs.