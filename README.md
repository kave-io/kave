# Kave

Kave is a tenant-scoped admission, accounting, and provider gateway for AI
workloads. The default runtime is deliberately small: one pure-Go server, one
PostgreSQL database, one Connect API, and an embedded read-focused console.

The kernel provides:

- forced PostgreSQL row-level isolation for every namespace;
- static agents, scoped service keys, and explicit capabilities;
- atomic quota and budget admission, reservation, and settlement;
- encrypted provider credentials and live route activation;
- OpenAI-compatible Responses, Chat Completions, and Embeddings gateways;
- immutable usage and audit ledgers without prompt or response persistence;
- bounded health and Prometheus surfaces without tenant-valued labels; and
- a compact console for observability, analytics, tenants, and audit evidence.

## Repository

```text
core/v2/                    domain contracts and validation
server/internal/v2/         PostgreSQL kernel, gateway, and HTTP services
proto/kave/kernel/v2/       canonical Connect contract
proto/gen/                  generated Go contract
dashboard/                  embedded production console
docs/v2-operations.md       deployment and security guide
```

## Build and test

Go 1.26.5, Bun 1.3.10, Buf 1.69, and ripgrep are the local prerequisites.
PostgreSQL 18 is required for database tests and deployments. Install the
pinned protobuf, analysis, and security helpers with `make tools`.

```sh
bun install --cwd dashboard --frozen-lockfile
make tools
make build
make test
make fmt-check vet
```

`make build` writes the statically linked server to `dist/kave-server`. The
same binary runs the server and the terminating `migrate`, `bootstrap`,
`healthz`, and `version` commands.

## Local stack

The included Compose stack publishes PostgreSQL and Kave only on loopback. It
does not contain reusable secrets. Generate two independent development keys:

```sh
cp .env.example .env
openssl rand -hex 32
openssl rand -hex 32
```

Put one result in each `.env` field, then start the database, migration job,
and server:

```sh
docker compose up --build --wait
curl -fsS http://127.0.0.1:8080/readyz
```

Create an initial namespace and service key through the host's loopback
database port:

```sh
make build-server
install -d -m 0700 .kave/bootstrap
export KAVE_RUNTIME_POSTGRES_DSN='postgres://kave_runtime@127.0.0.1:5432/kave?sslmode=disable'
export KAVE_RUNTIME_POSTGRES_ROLE='kave_runtime'
export KAVE_BOOTSTRAP_ACCOUNT='account/local'
export KAVE_BOOTSTRAP_APPLICATION='example'
export KAVE_BOOTSTRAP_ENVIRONMENT='development'
export KAVE_BOOTSTRAP_KEY_NAME='initial-admin'
export KAVE_BOOTSTRAP_OUTPUT="$PWD/.kave/bootstrap/initial-admin.key"
dist/kave-server bootstrap
```

The console is available at `http://127.0.0.1:8080/`. Use the generated key
and namespace ID it reports. The complete bootstrap and production handoff
contract is in the [operator guide](docs/v2-operations.md).

## Production image

```sh
docker build --build-arg VERSION=dev -t kave:dev .
docker run --rm kave:dev version
```

The final image is distroless, non-root, read-only compatible, and contains
only `kave-server`, CA certificates, and the embedded console. Release tags
publish signed Linux archives, SBOMs, checksums, and one multi-architecture
container image.

See [operations](docs/v2-operations.md) and the
[persistence design](docs/design/v2/00-kernel-persistence.md) before deploying.

## License

Apache 2.0. See [LICENSE](LICENSE).
