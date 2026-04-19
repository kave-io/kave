# Plan 01 — gRPC Completion

**Goal:** finish the gRPC surface so every v1 CLI operation has a corresponding unary RPC. The JSON/HTTP layer (Plan 02) will bridge to this — if it isn't here, the CLI can't call it.

## Background
- `app/runtime/runtime.go` has 3 RPCs returning `codes.Unimplemented`: `CancelRun`, `GetPriceBook`, `GetSpendReport`.
- `app/control/control.go:ListOrganizations` returns a hard-coded empty list.
- Every `List*` RPC in both services ignores the page cursor — only `req.Limit` is wired.
- Many control-plane ops required by the CLI (`docs/01-cli-spec.md`) have no RPC: policy delete/export/validate/test, agent restore/export, credential delete/test, budget set/get/unset, apply/diff.

## Scope (single session)
Implement the missing RPCs + cursor pagination. Do **not** add HTTP handlers — Plan 02 handles that. Do **not** add auth/RBAC — Plan 06.

### 1. Fix existing unimplemented
- `Server.CancelRun`: update run status → `cancelled`, set `EndedAt`, call `UpdateRun`.
- `Server.GetPriceBook`: call `appStore.GetPriceBook(ctx)` and map via `app/runtime/mappers.go` (add `priceBookToProto`). Keep the proto shape aligned with `core/model/runtime.PriceBook`.
- `Server.GetSpendReport`: call `appStore.GetSpendReport(ctx, filter)`, map via new `spendReportToProto`. Mirror the field mapping already in `server/api/cost.go`.

### 2. Organizations
- Add `ListOrgs(ctx, page) (PageResult[*Organization], error)` to `core/store.OrgStore`.
- Implement it in each store backend under `server/internal/store/*` (match the pattern used by `ListProjects`).
- Wire `app/control/control.go:ListOrganizations` to the new store method.

### 3. Cursor pagination for List* RPCs
- Change all `List*Request` protos in `proto/kave/{control,runtime}/v1/*.proto` to add `string cursor = N;` and `List*Response` to add `string next_cursor = N;`. Run `buf generate` (or `make proto` if the target exists).
- Update mappers (`pageFromProto`, response building) to read `req.Cursor` and set `resp.NextCursor = result.NextCursor`.
- Touch every `List*` in both services. None should drop `NextCursor` on the floor.

### 4. New control-plane RPCs
Add these to `proto/kave/control/v1/` and implement in `app/control/control.go`. Use existing store methods where they exist; otherwise add minimal store methods following the existing interface style (see `core/store/app_store.go`).

Required:
- `DeletePolicy(id) -> Empty` (soft delete via status="archived" if no hard delete exists).
- `ExportPolicy(id) -> PolicyYAML` (return the policy as a canonical YAML string; use `sigs.k8s.io/yaml` or `gopkg.in/yaml.v3`).
- `ValidatePolicy(yaml) -> ValidatePolicyResponse{ok, issues[]}` — shape-check only; reuse the struct tags.
- `RestoreAgent(id) -> Agent` — already on the store (`RestoreAgent`), just expose RPC.
- `DeleteCredential(id) -> Empty` — already on the store.
- `CreateBudget / GetBudget / DeleteBudget` on a new `BudgetStore` (scope: one record per agent, store in same backend as policies; id prefix `bgt`).

Defer (put a TODO comment, not a stub): apply/diff engine, policy test, credential test, connector ops — these need design and are split into later plans.

## Files to touch
- `proto/kave/control/v1/*.proto`, `proto/kave/runtime/v1/runtime.proto`
- `app/control/control.go`, `app/control/mappers.go`
- `app/runtime/runtime.go`, `app/runtime/mappers.go`
- `core/store/*.go` (new store methods & BudgetStore)
- `server/internal/store/<backend>/*.go` (implement new methods — match existing backend patterns)
- `core/model/control/budget.go` (new file if no budget type exists)

## Acceptance
- `go build ./...` clean in every module (`core`, `app`, `connectors`, `server`).
- `go test ./...` in `server/` passes.
- All three previously-unimplemented RPCs return real data; no `codes.Unimplemented` remains in `app/{control,runtime}/*.go`.
- A `List*` call with a cursor returned from a previous call yields the next page (add one table-driven test in `app/runtime/` or `app/control/` exercising a 2-page list).
- No `uuid.NewString()` introduced. Use `core/pkg/ids.New(prefix)`.

## Size estimate
~15 files, ~600 LOC. Fits one haiku session if the agent stays narrow — tell it to stop after acceptance criteria and not to touch HTTP.
