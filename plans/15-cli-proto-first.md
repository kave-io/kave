# 15 — CLI Proto-First Transport Migration

## Goal

Restore the "one proto contract" model. The CLI should consume the proto
contract directly via generated gRPC clients for any command that maps to an
existing RPC. The HTTP bridge stays, but only as an adapter for a small,
explicit set of bridge-only commands — not as the CLI's primary API surface.

Server transport is unchanged. This is a CLI-side refactor.

## Non-goals

- No new proto RPCs in this phase.
- No removal of the HTTP bridge.
- No dashboard/SDK changes.
- No rewrite of commands that have no matching RPC yet (see bridge-only list).

## Split rule

- **gRPC**: runtime, observability, and resource CRUD that SDKs / dashboard
  also consume (traces, spans, policies, credentials, agents, connectors).
- **HTTP bridge**: config-plane, one-shot operator commands, and local CLI
  state (auth, rbac, budget, price, config, ctx, admin, events, version,
  completion). These have no SDK consumers and extending proto for them
  is cost without benefit.

## Shape

One user-facing endpoint. The CLI picks the transport per command internally.

- Keep a single `--server` flag and single `server` field in context config.
  It points at the daemon. Users do not learn a second flag.
- The daemon exposes both gRPC and HTTP bridge. The CLI derives both from the
  one endpoint (same host; gRPC and HTTP ports come from daemon config /
  sensible defaults). No `--grpc-server`.

### Endpoint resolution

Default is the local daemon; config layers override for cloud access.
Precedence, lowest to highest:

1. Built-in default: `grpc://localhost:<daemon-grpc-port>` (zero-config
   local dev). The HTTP bridge default is derived from the same host.
2. User config file (`~/.config/kave/config.yaml`).
3. Active context (`kave ctx use <name>`) — this is the cloud-access layer.
4. `KAVE_SERVER` env var.
5. `--server` flag.

Canonical endpoint form is a single URL string with scheme: `grpc://` or
`grpcs://` (TLS). One format across all layers — do not accept a second
shape for cloud contexts.
- Introduce a `Transport` abstraction in `cli/internal/runtime` with two
  implementations:
  - `grpcTransport` — wraps a cached `*grpc.ClientConn` and exposes the
    generated `ControlPlaneServiceClient` / `RuntimeServiceClient`.
  - `httpTransport` — the current `http.Client` bridge path, unchanged.
- Auth is shared: the existing session-token store feeds both transports.
  gRPC sends it as `authorization: Bearer <token>` metadata via a
  `PerRPCCredentials` impl. No second auth path.
- Command handlers depend on the transport interface, not on `http.Client`.
  Proto-backed handlers call the gRPC client; bridge-only handlers keep
  calling the HTTP path.

## First wave (migrate these, in this order)

Each item is one PR. Land the harness with item 1, then reuse it.

1. **`trace`** (`get`, `list`) — maps to `RuntimeService` run/span reads.
   Land the bufconn-based gRPC test harness here (`cli/internal/testutil`).
2. **`span`** (`get`, `list`) — same service, reuses the harness.
3. **`policy`** (`get`, `list`, `apply`/`set`) — `ControlPlaneService` policy RPCs.
4. **`credential`** (`get`, `list`, `set`, `delete`) — control-plane credential RPCs.
5. **`agent`** (`get`, `list`, `apply`) — control-plane agent RPCs.
6. **`connector`** (`get`, `list`) — control-plane connector RPCs if present;
   skip verbs without an RPC.

Out of scope (stays on HTTP bridge permanently per the split rule):
`auth`, `rbac`, `config`, `ctx`, `budget`, `price`, `admin`, `events`,
`version`, `completion`. Streaming commands (`lifecycle watch`,
`lifecycle logs`) also stay on the bridge until a later gRPC-streaming
phase.

## Test harness

Add `cli/internal/testutil/grpcharness.go`:

- Spin up the real server gRPC services over `bufconn`.
- Return a `Transport` wired to that connection.
- Seed minimal fixtures (sqlite in-memory is fine if the service needs it).

Each migrated command gets at least one integration test using this harness
that asserts: (a) the RPC is called with the expected request, (b) output
formatting matches the existing HTTP-path golden output, (c) auth metadata
is forwarded.

## Acceptance criteria

- `cli/internal/runtime` exposes a `Transport` interface; gRPC and HTTP
  implementations both satisfy it.
- First-wave command handlers no longer import `net/http` directly.
- Existing `--server` UX unchanged; no new flags.
- Session token works identically over both transports.
- Bridge-only commands still pass their existing tests unchanged.
- New bufconn harness covers at least one command per migrated group.
- `docs/cli-transport.md` (see below) exists and is accurate.

## Risks / watch-outs

- **Endpoint derivation.** Canonical form is `grpc://host:port` or
  `grpcs://host:port`. HTTP bridge target is derived from the same host
  with the daemon's bridge port (config field, sensible default). Reject
  legacy `http(s)://` forms with a clear error pointing users at
  `kave ctx` migration.
- **Streaming.** `WatchRuns` is a server-streaming RPC. Do not migrate
  `lifecycle watch` / `logs` in this wave — they currently use the HTTP
  bridge's SSE path and are explicitly bridge-only (see doc below).
- **Output parity.** Existing tests assert exact CLI output. Migrated
  handlers must produce byte-identical output unless a test is updated
  with justification.
- **Do not add a proto RPC just to migrate a command.** If it is not
  already in proto, it stays on the bridge this phase.

## Order of operations for the implementing agent

1. Read `cli/internal/runtime/http.go`, `cli/internal/runtime/contract.go`,
   `cli/internal/runtime/runtime.go`, `cli/internal/config/*`.
2. Define `Transport` interface and refactor `Runtime` to hold one.
3. Implement `grpcTransport` with cached conn + bearer-token
   `PerRPCCredentials`.
4. Add `cli/internal/testutil` bufconn harness.
5. Migrate `trace` commands end-to-end with tests. Stop. Get review.
6. Only then proceed to items 2–6 of the first wave, one PR each.

---

# docs/cli-transport.md (short)

> Default: **gRPC-first.** Any CLI command not listed below talks to the
> daemon over gRPC using the generated proto client.

## HTTP bridge only (always)

Config-plane and local CLI state. No SDK consumers, no runtime hot path,
so extending proto would be cost without benefit.

- `auth` (login, logout, whoami) — session/browser flow is HTTP-native.
- `rbac` — operator admin surface.
- `config`, `ctx` — local CLI state, no daemon RPC.
- `budget`, `price` — one-shot config commands, not runtime.
- `admin` — operator endpoints.
- `events` — bridge-only.
- `version`, `completion` — local.

## Streaming (SSE over HTTP bridge, for now)

These are streaming commands that currently use SSE. They will stay on
the bridge until we migrate them to gRPC server-streaming in a later phase:

- `lifecycle watch`
- `lifecycle logs`

## Everything else → gRPC

`trace`, `span`, `policy`, `credential`, `agent`, `connector`, `apply`
(for proto-backed resource kinds). If a command is not in the two lists
above, it must go through the gRPC transport.
