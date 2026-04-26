# CLI Transport

Default: **gRPC-first.** Any CLI command not listed below talks to the
daemon over gRPC using the generated proto client.

## Endpoint configuration

One `--server` flag; one `server` field in context config. Points at the daemon.
The daemon exposes both gRPC and HTTP bridge. The CLI derives both transports
from the single endpoint.

Canonical form: `grpc://host:port` or `grpcs://host:port` (TLS).
Legacy `http(s)://` forms are rejected with a migration hint — use `kave ctx` to update.

Precedence (lowest to highest):
1. Built-in default: `grpc://127.0.0.1:9090` / HTTP bridge on `:8080`
2. User config file (`~/.kave/kave.yaml`)
3. Active context (`kave ctx use <name>`)
4. `KAVE_SERVER` env var
5. `--server` flag

## HTTP bridge only (always)

Config-plane and local CLI state. No SDK consumers, no runtime hot path.

- `auth` (login, logout, whoami) — session/browser flow is HTTP-native
- `rbac` — operator admin surface
- `config`, `ctx` — local CLI state, no daemon RPC
- `budget`, `price` — one-shot config commands, not runtime
- `admin` — operator endpoints
- `events` — bridge-only
- `version`, `completion` — local

## Streaming (SSE over HTTP bridge, for now)

These use SSE and stay on the bridge until a later gRPC-streaming phase:

- `lifecycle watch`
- `lifecycle logs`

## Everything else → gRPC

`trace`, `span`, `policy`, `credential`, `agent`, `connector`, and `apply`
(for proto-backed resource kinds). If a command is not in the two lists
above, it goes through the gRPC transport.
