# Plan 12 — Dashboard Sync: Catch the UI up to the Post-`core/` Data Model

**Goal:** the Vue 3 dashboard (embedded via `go:embed` in `server/ui/`) still reflects the pre-refactor data model. The data layer now exposes Org/User/Membership/Project/Environment, `money.Amount` as `{amount,currency}`, run provenance fields, and a real trace tree. Surface these in the UI without changing the server contract.

## Read first

- `dashboard/src/` — Vue 3 + TanStack Query, Biome-formatted. Entry points: `App.vue`, `main.ts`, `layouts/`, `components/`.
- `dashboard/src/data/` — API clients per noun. Currently speaks the old shape (Workspace, float dollars, flat spans).
- `server/internal/contract/` — source of truth for JSON shapes. Generate TS types from here (see §3).
- `server/internal/httpbridge/routes.go` — complete endpoint list. Anything not consumed today is a UI gap.
- `core/model/runtime/span.go`, `core/model/control/*.go` — the new Go types. Mirror in TS.
- `server/ops/trace/tree.go` — tree shape `{span, children}`.

## Design

### 1. Type generation

Stop hand-maintaining `dashboard/src/types/*.ts`. Add a small Go program `cmd/tsgen/main.go` that walks the contract types and emits TS interfaces. Run via `make tsgen` → `dashboard/src/types/generated.ts`. Check the generated file in; regen on contract change.

Why: today the front-end drifts silently when the server changes shape. A codegen step turns drift into a compile error.

### 2. Data-model sync

Rename and rewire:
- `Workspace` → `Project` everywhere (components, composables, routes).
- Add `Organization`, `User`, `Membership`, `Environment` resources with list+detail views under a collapsible nav group.
- Every list that filters by Project also accepts an Environment filter. Default is "all envs" with env chip on each row.
- All money fields render via a shared `<Money :amount :currency />` component that formats nano-dollars to human units at render time. Delete any `formatDollars(number)` helpers — they assume float and the wrong unit.

### 3. Trace view

New route `/traces/:id`:
- Fetches `GET /api/v1/traces/:id` → renders the tree. Left pane: collapsible tree; right pane: selected span details (attrs, timings, cost, errors).
- "Export" dropdown hits `/export?format=mermaid|dot|jsonl|otlp|parquet` and triggers a download.
- Trace list view `/traces` with cursor pagination, filters by agent / env / hasError / cost floor / date range — wire the expanded `SpanFilter`.

### 4. Run provenance

Run detail view adds a "Provenance" section showing `TriggerType`, `TriggerID`, `CorrelationID`, `SessionID`, `IdempotencyKey`. Cancelled / timed_out / blocked statuses render as distinct badges (not all under "failed").

### 5. Live SSE

Existing SSE subscription remains. Update the decoder to the JSONL envelope from plan 03 (typed event bus) — reuse the server `contract.stream` types. Confirm LIVE badge still flips correctly.

### 6. Auth UI (minimal)

Self-host dashboard has been cookie-less. Add a tiny login form that POSTs `/api/v1/auth/login`, stores the returned PASETO in `sessionStorage`, attaches as `Authorization: Bearer` on all fetches. When `AllowAnonymous=true` (v1 default), skip login entirely.

### 7. Polish without scope creep

Do not:
- Redesign the visual language.
- Add a settings page beyond "current identity / logout".
- Add Org management UI (deferred to cloud).

Do:
- Fix broken screens caused by renamed fields.
- Render new statuses.
- Wire trace view (the biggest visible v1 win).

## Files

Create:
- `cmd/tsgen/main.go` + `Makefile` target.
- `dashboard/src/components/trace/TreeView.vue`, `SpanDetail.vue`.
- `dashboard/src/components/common/Money.vue`.
- `dashboard/src/views/traces/List.vue`, `Detail.vue`.
- `dashboard/src/data/traces.ts`, `environments.ts`, `organizations.ts`.

Modify (substantial):
- `dashboard/src/data/*.ts` — switch to generated types, rename Workspace→Project.
- `dashboard/src/components/run/*.vue` — provenance + new statuses + money.
- `dashboard/src/layouts/Sidebar.vue` — new nav groups.
- Router to include `/traces/*` and `/environments/*`.

Delete:
- Any `types/workspace.ts` / `formatDollars*` / legacy cost utilities.

## Acceptance

- `bun run typecheck` green (TS strict).
- `bun run build` under the existing bundle budget (ci check: no >10% regression).
- Manual: `kave start` → dashboard loads → can view runs with new statuses, open a run, click into its trace, see the tree, export mermaid — and the downloaded file renders as a valid sequenceDiagram in https://mermaid.live.
- Live SSE still flips LIVE badge within 2s of a new event.
- Login flow works when `AllowAnonymous=false`; screen is hidden when `true`.
- `make tsgen` is idempotent (no diff if server contract hasn't changed).

## Out of scope
- Org/team management UI (cloud).
- Role editor (cloud).
- Cost forecasting UI (post-v1).
- Replay sandbox (post-v1).
- Deep customizability, themes beyond existing light/dark.

## Size estimate
~1200 LOC (TS/Vue) + 300 LOC (tsgen Go). One haiku session if it delivers in order: tsgen → rename → trace views → provenance → login toggle.
