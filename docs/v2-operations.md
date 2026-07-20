# Kave operator guide

Kave is a PostgreSQL-only, machine-to-machine kernel. Its serving process does
not migrate schemas, mint a standing bootstrap credential, discover config
files, or fall back to host-local secrets. Configuration is environment-only.

The supported deployment order is:

1. create a migration owner and a separate runtime login;
2. run the terminating `kave-server migrate` job;
3. run `kave-server bootstrap` once for each initial namespace;
4. move the generated admin key into a secret manager;
5. start `kave-server serve` with the runtime login; and
6. replace the bootstrap admin with narrowly scoped workload and reporting
   keys.

Never put a provider credential, service key, encryption key, or database
password in a manifest, command argument, source repository, or log field.

## Database roles and migration

Use a migration login that can `SET ROLE`, a `NOLOGIN` schema owner, and a
direct runtime `LOGIN` with no privileged memberships. For a database named
`kave`:

```sql
CREATE ROLE kave_owner
  NOLOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE
  NOREPLICATION NOBYPASSRLS;

CREATE ROLE kave_runtime
  LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE
  NOREPLICATION NOBYPASSRLS PASSWORD '<from secret manager>';

GRANT CREATE ON DATABASE kave TO kave_owner;
GRANT CONNECT ON DATABASE kave TO kave_runtime;
GRANT kave_owner TO kave_migrator;
```

The runtime login must not own the database, schema, tables, sequences, or
functions. It must not inherit the owner, be a superuser, or have `BYPASSRLS`.
Kave verifies the login identity, forced-RLS posture, object ownership,
memberships, and exact grants before serving traffic. Unexpected access fails
startup.

Run migrations as a terminating deployment job:

```sh
export KAVE_MIGRATION_POSTGRES_DSN='postgres://kave_migrator:...@db.example/kave?sslmode=verify-full'
export KAVE_MIGRATION_OWNER_ROLE='kave_owner'
export KAVE_RUNTIME_POSTGRES_ROLE='kave_runtime'
kave-server migrate
```

Remote DSNs must require TLS without plaintext fallback. The migration login
exists only in this job. Run the job before every binary rollout. Embedded
migrations are checksum-pinned and serialized by an advisory lock; a checksum
mismatch is a hard failure, not a reason to edit an applied migration.

## Offline bootstrap

Bootstrap uses the verified runtime login and writes a client-generated raw
service key to a new `0600` file. Kave stores only its lookup prefix and
one-way verifier. The raw value is never printed or recoverable from
PostgreSQL.

Prepare an absolute output directory that is not group- or world-writable:

```sh
install -d -m 0700 /run/kave-bootstrap

export KAVE_RUNTIME_POSTGRES_DSN='postgres://kave_runtime:...@db.example/kave?sslmode=verify-full'
export KAVE_RUNTIME_POSTGRES_ROLE='kave_runtime'
export KAVE_BOOTSTRAP_ACCOUNT='account/acme'
export KAVE_BOOTSTRAP_APPLICATION='checkout'
export KAVE_BOOTSTRAP_ENVIRONMENT='production'
export KAVE_BOOTSTRAP_KEY_NAME='initial-admin'
export KAVE_BOOTSTRAP_OUTPUT='/run/kave-bootstrap/initial-admin.key'

kave-server bootstrap
```

The output must not already exist. Kave reserves it with create-exclusive
semantics, writes and syncs the raw key before mutating the database, and
syncs the parent directory. On an ambiguous issuance result the durable file
is retained so the exact idempotent request can be checked. Move it to a
secret manager, verify the stored value, and remove the handoff file according
to local policy.

Bootstrap grants `config.apply`, `secrets.write`, `keys.manage`,
`limits.sync`, `usage.read`, and `audit.read`. It cannot consume quota, invoke
an agent, or assert workload scope. Use it only to provision replacements,
then revoke it.

## Serving configuration

`kave-server` and `kave-server serve` are equivalent. The following variables
form the complete serving contract:

| Variable | Required | Meaning |
| --- | --- | --- |
| `KAVE_SERVER_ADDR` | no | HTTP bind; default `127.0.0.1:8080` |
| `KAVE_RUNTIME_POSTGRES_DSN` | yes | direct runtime-login PostgreSQL DSN |
| `KAVE_RUNTIME_POSTGRES_ROLE` | yes | exact expected session login |
| `KAVE_RUNTIME_MASTER_KEY` | yes | active 32-byte encryption key, hex or base64 |
| `KAVE_RUNTIME_MASTER_DECRYPTION_KEYS` | no | comma-separated previous encryption keys, with no spaces |
| `KAVE_RUNTIME_SECRET_IDEMPOTENCY_KEY` | yes | independent stable 32-byte idempotency key |
| `KAVE_RUNTIME_TRANSPORT_SECURITY` | yes | `tls_terminated`, `private_network`, or `development` |
| `KAVE_RUNTIME_PROVIDER_ALLOWED_PRIVATE_IPS` | no | comma-separated exact private provider IP exceptions |
| `KAVE_RUNTIME_READINESS_TIMEOUT` | no | dependency timeout, default `3s`, range `1s`–`30s` |
| `KAVE_RUNTIME_SHUTDOWN_TIMEOUT` | no | request-drain timeout, default `30s`, range `1s`–`5m` |

Example systemd or orchestrator environment:

```sh
KAVE_SERVER_ADDR=0.0.0.0:8080
KAVE_RUNTIME_POSTGRES_DSN=postgres://kave_runtime:...@db.example/kave?sslmode=verify-full
KAVE_RUNTIME_POSTGRES_ROLE=kave_runtime
KAVE_RUNTIME_MASTER_KEY=<secret-manager injection>
KAVE_RUNTIME_SECRET_IDEMPOTENCY_KEY=<different secret-manager injection>
KAVE_RUNTIME_TRANSPORT_SECURITY=tls_terminated
```

Transport modes are explicit trust assertions:

- `tls_terminated` permits any bind and requires a trusted TLS proxy or service
  mesh to be the only public path;
- `private_network` requires a private or loopback IP bind; and
- `development` requires a loopback bind.

Service keys are bearer credentials. Do not expose plaintext Kave traffic or
publish a private/development listener through a public load balancer.

## Encryption-key rotation

The active master encrypts new provider secret versions. Previous masters may
decrypt older versions. The idempotency key authenticates write-only replay
records and must remain independent and stable.

Rotate in this order:

1. add the old master to `KAVE_RUNTIME_MASTER_DECRYPTION_KEYS`;
2. install the new master as `KAVE_RUNTIME_MASTER_KEY`;
3. restart and verify existing routes can still be prepared;
4. write a new version of every encrypted provider secret;
5. reactivate each route against its new secret version; and
6. remove the old decryption key only when it is no longer needed.

Changing the idempotency key makes existing secret-write replay records
unverifiable.

## Provisioning and provider activation

After offline bootstrap has established the namespace and returned its ID,
provision its runtime configuration in this order:

1. `PutSecret` an encrypted provider credential;
2. `Apply` a manifest with routes, static agents, and operator limits;
3. call `ActivateProviderRoute` for each route/model that should receive live
   traffic;
4. issue workload keys with only `consume` and/or `invoke`, an explicit agent
   allowlist, and scope assertion enabled;
5. issue separate reporting and control keys; and
6. revoke the bootstrap key.

An account-scoped bootstrap integration that has authority before a namespace
exists follows the same invariant explicitly: `Apply` a namespace-only
manifest, write the encrypted secret into the returned namespace, then `Apply`
the full route/agent manifest and activate the route. Kave does not persist a
route with an unresolved secret reference.

`Apply` is transactional and idempotent. Routes are persisted as invalid until
activation performs a provider-specific, payload-free live credential/model
check. The validation result is bound to the exact route revision, secret
version, and model; concurrent changes make activation stale. Failure records
bounded evidence and leaves the route unavailable. Provider bodies and
credentials are never retained in validation evidence.

A route admits only models with successful activation evidence for its exact
current encrypted-secret version. Evidence is retained per allowed model in a
bounded route document; a failed revalidation removes that model while leaving
independently validated models available. Any route-topology, model-policy, or
credential change clears the complete set and fails closed until reactivation.

The built-in adapter supports OpenAI and explicitly configured
OpenAI-compatible providers. Validation uses an authenticated model lookup.
The gateway supports Responses, Chat Completions, and Embeddings, including
streamed responses. It parses reported input, output, cache-read, cache-write,
and reasoning tokens for immutable settlement. Missing detailed counters are
zero quantities; an omitted detailed price inherits the corresponding normal
input/output rate. Malformed, contradictory, or wrong-model usage is treated
as unreported and settles matching limits at their conservative reservations,
with estimated provenance, rather than silently undercharging.

Every allowed model requires a non-negative price snapshot and a positive
pricing revision. Prices are nano-USD per million tokens. Changing a price
requires a higher revision so historical usage retains the exact admission
and settlement snapshot.

Kave does not persist prompts or provider responses. Application identities,
emails, subscriptions, or regulated data do not belong in a manifest. Use
opaque tenant, actor, billing, session, and feature references at request time.

## Capabilities

| Capability | Surface |
| --- | --- |
| `config.apply` | `Apply`, `GetState`, and route activation |
| `secrets.write` | provider secret writes and revocation |
| `keys.manage` | service-key issuance and revocation |
| `limits.sync` | entitlement limit synchronization |
| `usage.read` | tenant, usage, invocation, and limit-status reporting |
| `audit.read` | namespace audit reporting |
| `consume` | exact product-quota admission for allowed agents |
| `invoke` | provider gateway traffic for allowed agents |

`consume` and `invoke` require a non-empty immutable agent-ID allowlist and
scope assertion. Reporting is namespace-bound. Usage and invocation queries
also require exact tenant and billing references; tenant summaries expose only
opaque scope references and aggregate operational values.

For a production console, issue a reporting key with `usage.read` and
`audit.read`. Add `config.apply` only when manifest inspection is required.
The console contains no mutation controls, sends keys only to its own origin,
keeps them in memory by default, optionally uses tab-scoped `sessionStorage`,
and never uses `localStorage`. Serve it only through HTTPS and protect access
with the same network or identity boundary used for operational dashboards.

## Health, metrics, and logs

| Endpoint | Purpose |
| --- | --- |
| `GET /livez` | process liveness; no dependency checks |
| `GET /readyz` | runtime role, migrations, database, and keyring readiness |
| `GET /metrics` | Prometheus text exposition |

The image health check runs `kave-server healthz`, which probes `/readyz`.
`KAVE_HEALTH_URL` may change the scheme or port only; it must remain an exact
`/readyz` URL on a loopback IP literal and cannot contain credentials, a query,
or a fragment. Readiness therefore cannot be delegated to an unrelated host.

Metrics use fixed outcome, operation, method, status-class, and surface labels.
Tenant references, keys, invocation IDs, routes, provider request IDs, and
models never become labels. Readiness responses omit dependency errors. Logs
are structured JSON and must still be collected as sensitive operational
data. Restrict `/metrics` to the Prometheus network even though its label
surface is bounded.

Alert at minimum on sustained readiness failure, authentication-unavailable
outcomes, admission/accounting failures, uncertain provider settlements,
provider validation failures, elevated HTTP 5xx rates, and PostgreSQL pool
exhaustion.

## Provider egress

Provider requests ignore ambient `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY`.
For every new connection Kave resolves the provider name, rejects the complete
answer if any address is private, loopback, link-local, multicast,
unspecified, reserved, documentation/benchmark space, or instance metadata,
and dials only an approved IP literal while preserving the original HTTP Host
and TLS server name.

Production should leave `KAVE_RUNTIME_PROVIDER_ALLOWED_PRIVATE_IPS` empty. A
self-hosted provider is an explicit exception containing exact canonical
loopback, RFC 1918, or IPv6 unique-local addresses. CIDRs and blanket
allow-private flags are unsupported. Link-local, multicast, unspecified,
reserved, and metadata addresses cannot be excepted. Plain HTTP provider URLs
are restricted to loopback.

## Backup, recovery, and upgrades

Back up PostgreSQL with point-in-time recovery and protect the active master,
old decryption keys still in use, and the stable idempotency key independently.
A database backup without its required decryption keys cannot recover provider
credentials. Raw service keys are intentionally absent from the database and
must be recovered from each recipient's secret manager or replaced.

For an upgrade:

1. back up the database and verify key availability;
2. run the new image's `migrate` job;
3. start a canary with the runtime role;
4. require `/readyz` success and inspect bounded metrics;
5. roll the remaining instances; and
6. verify route activation and accounting on representative workloads.

The serving process never migrates automatically. Rollback is a binary
decision only when the older binary understands the already-applied schema;
otherwise restore according to the migration's documented recovery path.

## Verify a release

Release tags publish only versioned archives and images; there is no mutable
`latest` image. Verify an archive, its Sigstore bundle, and its GitHub build
provenance before installation:

```sh
version=v2.0.0
gh release download "$version" --repo kave-io/kave --dir "kave-$version"
cd "kave-$version"
sha256sum --check checksums.txt
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/kave-io/kave/.github/workflows/release.yml@refs/tags/$version" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
gh attestation verify kave-server_*.tar.gz --repo kave-io/kave
```

Resolve the published image to a digest, then verify that exact digest rather
than trusting a local tag:

```sh
image=ghcr.io/kave-io/kave
digest="$(docker buildx imagetools inspect "$image:$version" --format '{{json .Manifest.Digest}}' | tr -d '"')"
cosign verify \
  --certificate-identity "https://github.com/kave-io/kave/.github/workflows/release.yml@refs/tags/$version" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  "$image@$digest"
```
