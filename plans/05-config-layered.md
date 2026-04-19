# Plan 05 — Layered Config (`kave.yaml`) + Expansion + Watched Reload

**Goal:** replace the current single-file Viper load with the 5-layer `kave.yaml` contract from `docs/03-kave-yaml-config.md`: builtin → system → user → project → env. Add `${VAR}` expansion, discovery, watched-file reload with rollback-on-invalid, and an apply engine stub for `apply/diff` so the CLI's resource flow works end-to-end.

## Read first

- `docs/03-kave-yaml-config.md` is the ground truth. Implement its sections 1–6. Sections 7–10 (agents/policies/credentials under `project:`) connect to the existing control stores — the loader produces resource records; a separate applier syncs them.
- Current loader: `server/internal/config/{read.go,types.go}` — single-file Viper, env binding already works (keep). All layering lives in a new `layered.go`, leaving `ReadConfig` as a thin wrapper.
- `daemon.State.Reload` (Plan 04) already has the reload hook and the `RequiresRestart` partitioning. Extend rather than replace.
- Current Viper env binding produces `KAVE_PARENT_CHILD` mapping — keep it; it becomes layer 5.

## Design

### 1. New types for layered config
`server/internal/config/layered.go`:

```go
type Source string
const (
    SourceBuiltin Source = "builtin"
    SourceSystem  Source = "system"
    SourceUser    Source = "user"
    SourceProject Source = "project"
    SourceEnv     Source = "env"
)

type LayerFile struct {
    Source Source
    Path   string     // empty for builtin/env
    Raw    map[string]any
}

type LoadResult struct {
    Config *Config
    Layers []LayerFile    // ordered low→high
    Origin map[string]Source // dotted-path → layer that set the final value
}

func Load(opts LoadOpts) (*LoadResult, error)

type LoadOpts struct {
    ExplicitPath string  // --config / KAVE_CONFIG; when set, skips discovery
    StartDir     string  // cwd for project discovery
    Env          map[string]string // defaults to os.Environ (injectable for tests)
}
```

### 2. Merge semantics
- **Maps deep-merge.** Walk both trees; leaf wins by higher layer.
- **Lists replace wholesale.** Higher layer's list replaces lower — no element-level merge.
- **Resources identified by `name`.** For the four resource arrays (`agents`, `policies`, `credentials`, `connectors`), merge by name across layers: higher layer wins for a given name, otherwise accumulate. Implement as a small helper keyed off a static map `resourceListPaths = []string{"project.agents", "project.policies", "project.credentials", "project.connectors"}`.
- Record the winning source per dotted path in `Origin` — `kave config view` uses it.

### 3. Env expansion
`server/internal/config/expand.go`:

- Input: raw text of a YAML file.
- Syntax: `${VAR}`, `${VAR:-default}`, `${VAR:?reason}`, literal `$$VAR` → `$VAR`.
- Expansion happens **on each layer file before YAML parse** (textual, per-file), using the process env. Test with a table of cases.
- Errors on unset `${VAR}` or `${VAR:?reason}` — attach the layer path + line number to the error via a `ExpandError` type.

### 4. Discovery
- System: `/etc/kave/kave.yaml` on unix, `%PROGRAMDATA%\kave\kave.yaml` on windows (build-tagged helper).
- User: `~/.kave/kave.yaml` (via `os.UserHomeDir`, explicit — don't use `~` string).
- Project: walk up from `cwd`; stop at first `kave.yaml` or `kave.yml`, at `$HOME`, or at filesystem root (whichever first). Match the doc exactly.
- `--config` / `KAVE_CONFIG`: bypasses discovery, loads that one file at project layer.

### 5. Direct env overrides (layer 5)
Keep the existing `bindEnvs` but lift it to a final override stage applied after layer-file merge. The key set is authoritative — no ad-hoc env lookup elsewhere.

### 6. Watched reload
`server/internal/config/watch.go`:

- Use `github.com/fsnotify/fsnotify` (already transitively present via viper; add explicit dep if needed).
- Watch every discovered layer file (not the dir — file-level). Debounce 250ms.
- On change: re-run `Load`. If validation fails, log `warn: config reload rejected: <err>`, keep live config, do **not** swap. If valid, call `daemon.State.Reload` (Plan 04 already partitions applied vs requires-restart).
- Expose `(state *daemon.State).StartWatch(ctx)` — idempotent, cancelled by ctx.

### 7. Resource applier (for `kave apply` / `kave diff`)
The config carries `project.agents|policies|credentials|connectors`. Make the daemon reconcile these into the control stores on boot and on reload.

`server/internal/daemon/apply.go`:

```go
type ApplyPlan struct {
    Creates []ResourceOp
    Updates []ResourceOp
    Deletes []ResourceOp
}
type ResourceOp struct {
    Kind   string  // "agent"|"policy"|"credential"|"connector"
    Name   string
    Before any     // nil on create
    After  any     // nil on delete
    Source Source  // which layer declared it
}

func (s *State) BuildPlan(ctx context.Context) (ApplyPlan, error)
func (s *State) Apply(ctx context.Context, plan ApplyPlan, prune bool) (ApplyReport, error)
```

- `BuildPlan`: diff declared resources (from `cfg.Project.*`) against the stores.
- `Apply`: create + update in dependency order (policy → credential → agent → connector). When `prune` is true, delete store resources not in config *where their originating Source is `project` or `user`* (never prune resources created via API that aren't in any file — those have no origin).
- Call it at boot after `seedDefaults` (so file-defined agents get materialized) and at the end of each successful `Reload`.

### 8. Bridge endpoints
Extend `daemon_routes.go`:

- `POST /api/v1/apply` → body `{prune bool}` → `Outcome{Kind:"ApplyReport"}`.
- `GET  /api/v1/diff`  → returns `ApplyPlan` without executing (this is the real `/api/v1/diff`, distinct from `/api/v1/config/diff`).
- `GET /api/v1/config/paths` → origin map `{path: source}` for debugging.

### 9. Rollback safety
- On reload: before swapping, run `BuildPlan` with the new config. If planning itself errors (e.g., dangling credential reference), reject the reload.
- Hot-swap the `cfg` pointer atomically after plan success; call `Apply` after the swap.
- If `Apply` partially fails, log the error and stop — do not auto-revert store writes (per the "deliberate, auditable changes" principle). The error surfaces in the reload response.

## Files

Create:
- `server/internal/config/layered.go`
- `server/internal/config/expand.go`
- `server/internal/config/watch.go`
- `server/internal/config/layered_test.go`, `expand_test.go`
- `server/internal/daemon/apply.go`
- `server/internal/daemon/apply_test.go`

Modify:
- `server/internal/config/read.go` — keep `ReadConfig` as thin wrapper over `Load` returning `*Config` for callers that still want the old shape; add `LoadResult` path for the daemon.
- `server/internal/config/types.go` — add `Project`, `Agents`, `Policies`, `Credentials`, `Connectors`, `Contexts` struct fields matching the doc (currently missing). Use existing `controlmodel` types where they fit; keep config types distinct where doc uses different keys (e.g. camelCase).
- `server/internal/daemon/daemon.go` — accept `LoadResult`, call `Apply` after boot and after reload, expose `Origin`.
- `server/internal/httpbridge/daemon_routes.go` — wire apply/diff/paths.
- `server/main.go` — swap `config.MustReadConfig` for `config.Load`, start watcher.

## Acceptance

- Integration test `layered_test.go`: three temp files at system/user/project, env overrides; asserts precedence + map-merge + list-replace + resource-by-name.
- `expand_test.go`: all four syntaxes + error cases + `$$VAR` escape.
- `GET /api/v1/config/view` returns the merged config; `GET /api/v1/config/paths` shows per-path origin. `daemon.fx.refresh_interval_seconds` origin flips from `project` → `env` when `KAVE_FX_REFRESH_INTERVAL_SECONDS=30` is set.
- `touch ./kave.yaml` with a new agent added triggers reload; new agent appears in `/api/v1/agents` without restart.
- `echo 'invalid: [' >> ./kave.yaml` produces a `warn: config reload rejected` log and leaves the live config intact.
- `POST /api/v1/apply -d '{"prune":true}'` reconciles. `GET /api/v1/diff` shows empty plan after a fresh apply.
- `kill -HUP <pid>` still works (Plan 04 hook unchanged).
- `go build ./... && go test ./...` clean across modules.

## Out of scope
- Full Windows system-path testing (leave `//go:build windows` stub, test on unix path in CI).
- Context-switching CLI (`kave ctx use ...`) — server only needs to expose `currentContext`; CLI owns the switching.
- Pricing overrides merge — copy the pricing overrides directly into the cost service via `Apply`; detailed price-book versioning is out.

## Size estimate
~900 LOC (loader 300, expand 120, watch 120, applier 200, tests 160). One haiku session if the agent builds in this order: types → loader/merge → expand → discovery → watch → applier → routes → tests.
