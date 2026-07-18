# Kave V2 operator guide

Kave V2 runs beside V1 while applications migrate. It is a Postgres-only,
machine-to-machine kernel: there is no human login, no persistent bootstrap
HTTP credential, and no automatic schema migration in the serving process.

The supported deployment sequence is:

1. create separate migration-owner and runtime database roles;
2. run the terminating `v2-migrate` command;
3. run the terminating `v2-bootstrap` command once for each initial namespace;
4. store the resulting admin service key in the application's secret manager;
5. configure and start the serving process with the runtime role;
6. replace the bootstrap admin with narrowly capable control, reporting, and
   workload keys.

Do not put a provider key, a Kave service key, a master key, or a database
password in a manifest, source repository, command argument, or log field.

## PostgreSQL roles and migration

Use three identities:

- a migration login that exists only in the migration job and can `SET ROLE`;
- a dedicated `NOLOGIN` V2 schema owner;
- a direct, dedicated V2 runtime `LOGIN` with no privileged memberships.

For a database named `kave`, an administrator can establish the roles with a
site-specific password and migration-login name:

```sql
CREATE ROLE kave_v2_owner
  NOLOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE
  NOREPLICATION NOBYPASSRLS;

CREATE ROLE kave_v2_runtime
  LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE
  NOREPLICATION NOBYPASSRLS PASSWORD '<from secret manager>';

GRANT CREATE ON DATABASE kave TO kave_v2_owner;
GRANT CONNECT ON DATABASE kave TO kave_v2_runtime;
GRANT kave_v2_owner TO kave_migrator;
```

The schema owner must remain distinct from the session login and must not be
able to log in. The runtime role must not own the database, schema, tables, or
functions; inherit the owner; be a superuser; or have `BYPASSRLS`. Kave checks
these invariants at runtime startup. It also rejects a runtime connection that
used `SET ROLE` instead of logging in directly. Verification is exact and
future-closed: every required DML grant and the
`kave_v2.lookup_service_key(text)` execution grant must exist, while an
effective grant on any other V2 table, view, sequence, or function prevents
startup. Running `v2-migrate` reconverges direct and `PUBLIC` object grants
before the runtime pool is opened.

Run migrations in a terminating job, never in the serving container:

```sh
export KAVE_V2_MIGRATION_DSN='postgres://kave_migrator:...@db.example/kave?sslmode=verify-full'
export KAVE_V2_OWNER_ROLE='kave_v2_owner'
export KAVE_V2_RUNTIME_ROLE='kave_v2_runtime'
kave-server v2-migrate
```

For a remote database, the DSN must require TLS without a plaintext fallback.
The command sets the dedicated owner role, applies checksum-pinned embedded
migrations under an advisory lock, and converges the runtime role to Kave's
exact table/function grants. The serving process never reads
`KAVE_V2_MIGRATION_DSN` or the owner role.

Run `v2-migrate` for every binary upgrade before rolling out that binary. A
migration checksum mismatch is a hard failure and must be investigated; do not
edit an already-applied migration or its registry row.

## Offline namespace bootstrap

`v2-bootstrap` uses the verified, least-privilege runtime login. It creates or
finds one `account + application + environment` namespace, then creates one
namespace-bound admin key with all six control/reporting capabilities:
`config.apply`, `secrets.write`, `keys.manage`, `limits.sync`, `usage.read`,
and `audit.read`. The initial key cannot consume quota, invoke an agent, or
assert request scopes. Split and replace it during operational handoff.

Prepare an existing operator-owned directory that is not writable by group or
other users. The final output file must not already exist:

```sh
install -d -m 0700 /run/kave-bootstrap

export KAVE_V2_RUNTIME_DSN='postgres://kave_v2_runtime:...@db.example/kave?sslmode=verify-full'
export KAVE_V2_RUNTIME_ROLE='kave_v2_runtime'
export KAVE_V2_BOOTSTRAP_ACCOUNT='account/acme'
export KAVE_V2_BOOTSTRAP_APPLICATION='simorq'
export KAVE_V2_BOOTSTRAP_ENVIRONMENT='production'
export KAVE_V2_BOOTSTRAP_KEY_NAME='initial-admin'
export KAVE_V2_BOOTSTRAP_OUTPUT='/run/kave-bootstrap/initial-admin.key'

kave-server v2-bootstrap
```

The output path must be clean and absolute. Kave reserves it with
create-exclusive semantics, forces mode `0600`, generates and durably writes
the raw key followed by one newline, and syncs the file and parent directory
before submitting its one-way verifier. The raw key never crosses the control
API or appears on stdout. On success, stdout contains only the namespace ID,
service-key ID, and output path. Move the file into the application's secret
manager, verify the stored value, then securely remove the handoff file
according to local policy.

Namespace and key idempotency identifiers are derived from the explicit
bootstrap inputs. This gives bootstrap the following retry behavior:

- If the output path already exists, bootstrap stops before any database
  operation and never truncates the file.
- Reapplying the namespace is safe and non-destructive.
- A write, sync, or close failure happens before namespace or key mutation and
  removes the incomplete reservation.
- An idempotent issuance replay succeeds because the preserved file supplies
  the same prefix and verifier; `Created == false` does not make its raw value
  unrecoverable.
- If issuance returns an error or inconsistent metadata, Kave preserves the
  durable `0600` output because the commit result may be ambiguous. Test that
  credential before deciding whether to retry with the same material or issue
  a deliberately named replacement.

This ordering removes the prior commit-before-delivery crash window. Raw keys
remain intentionally unrecoverable from Postgres, so loss of the output file
still requires a replacement and revocation of the inaccessible key.

Bootstrap is not available over HTTP. Do not add a bootstrap bearer token,
default agent, default namespace, or permissive authentication mode to work
around an operator error.

### Service-key contract compatibility

The V2 kernel is a new, pre-release contract, so service-key issuance does not
carry a server-generated compatibility mode. Every V2 issuance client must send
`lookup_prefix` and the 32-byte `secret_hash`; the former response field 4
(`raw_key`) is reserved and is never populated. This does not alter the V1
control API or V1 agent tokens. Upgrade the V2 server and V2 SDK together.

The credential recipient generates the canonical raw key and keeps it. The
offline bootstrap command performs that client role locally before database
mutation. Only the key's non-secret 24-character lookup component and SHA-256
verifier enter `IssueServiceKey`; the serving API neither generates nor returns
the raw credential, and Postgres cannot recover it. The raw key is later
presented as a bearer token to Kave's authentication boundary, but it must
never be sent as issuance payload, returned by a control response, or written
to logs. An issuance retry supplies the same prefix and verifier and safely
receives the same metadata.

### Capability model

V2 has no broad persisted `apply` or administrator capability. Grant the
narrow operations required by each machine identity:

| Capability | Permitted surface |
| --- | --- |
| `config.apply` | `Apply` and `GetState` |
| `secrets.write` | `PutSecret` and `RevokeSecret` |
| `keys.manage` | `IssueServiceKey` and `RevokeServiceKey` |
| `limits.sync` | `SyncLimits` |
| `usage.read` | scoped usage, invocation, and limit-status reads |
| `audit.read` | namespace audit-event reads |
| `consume` | exact product-quota consumption and allowed-agent limit status |
| `invoke` | allowed-agent provider gateway calls and limit status |

`consume` and `invoke` require a non-empty agent allowlist and
`can_assert_scope`. A scoped `GetLimitStatus` call also requires
`can_assert_scope`, whether authorized by `usage.read` or a workload
capability. Agent restrictions are stored as immutable agent IDs, not names.
Reporting queries remain namespace-bound; usage and invocation reads
additionally require exact tenant and billing-subject filters.

## Serving configuration

Inject sensitive values from the deployment secret manager into `kave.yaml`.
The configuration loader supports required environment expansion:

```yaml
v2:
  enabled: true
  runtime_dsn: "${KAVE_V2_RUNTIME_DSN:?required}"
  runtime_role: "kave_v2_runtime"
  transport_security: "tls_terminated"
  master_key: "${KAVE_V2_MASTER_KEY:?required}"
  master_decryption_keys: []
  secret_idempotency_key: "${KAVE_V2_SECRET_IDEMPOTENCY_KEY:?required}"
  provider_egress:
    # Empty in production: only globally routable provider IPs are accepted.
    allowed_private_ips: []
```

`runtime_dsn` must log in directly as `runtime_role`. The serving process
verifies its identity, RLS posture, object ownership, memberships, and exact
runtime grants before registering V2 routes.

Choose the HTTP transport boundary explicitly:

- `tls_terminated`: a trusted proxy or service mesh terminates TLS before Kave;
- `private_network`: Kave must bind a private or loopback IP;
- `development`: Kave must bind a loopback IP.

Service keys are bearer credentials. Never expose a `private_network` listener
through a public load balancer, and never use `development` outside a local
machine.

### Provider egress boundary

The V2 provider client does not use `HTTP_PROXY`, `HTTPS_PROXY`, or
`NO_PROXY`. On every new connection, Kave resolves the provider hostname,
rejects the complete DNS answer if any address is private, loopback,
link-local, multicast, unspecified, reserved, documentation/benchmark space,
or a known instance-metadata address, and dials only the validated IP literal.
The original hostname remains the HTTP Host and TLS server name. This prevents
a second resolver lookup from turning an approved hostname into an internal
destination.

Production deployments should leave `allowed_private_ips` empty. A local or
self-hosted provider is an explicit exception using exact canonical IP
literals, never a hostname, CIDR, or blanket allow-private setting:

```yaml
v2:
  provider_egress:
    allowed_private_ips:
      - "127.0.0.1"
      - "::1"
      # Or one exact RFC 1918/IPv6 unique-local address used by the provider.
```

Only loopback, RFC 1918, and IPv6 unique-local addresses can be excepted.
Link-local, multicast, unspecified, reserved, and known metadata addresses
remain forbidden even when listed. Add every exact address returned by a
multi-address private provider name; a mixed allowed/denied DNS response fails
closed. Plain HTTP route URLs remain restricted to loopback providers, so a
private self-hosted provider must normally serve HTTPS.

## Master keys and provider credentials

`master_key` and `secret_idempotency_key` each encode exactly 32 random bytes
as hex or base64. Use independent values from the first deployment. Kave does
not generate an OS-keyring key or a host-local fallback. Without a configured
keyring, encrypted secret writes and encrypted provider invocation cannot
succeed.

The local keyring uses envelope encryption. New secret versions use
`master_key`; `master_decryption_keys` may decrypt versions written under old
keys. The stable `secret_idempotency_key` authenticates write-only
idempotency records and must not change during encryption-key rotation.

Rotate a master key in this order:

1. retain the old master in `master_decryption_keys`;
2. install the new master as `master_key` while leaving
   `secret_idempotency_key` unchanged;
3. restart and verify old provider routes still work;
4. write a new version of each encrypted secret using a fresh request
   idempotency key, so it is wrapped by the new master;
5. remove the old decryption key only after every required encrypted secret
   has been rewritten and verified.

If an older deployment implicitly used its sole master as the idempotency
source, set `secret_idempotency_key` to that old encoded master before the
first rotation and keep it stable. Changing it makes old secret-write replay
records unverifiable.

Kave persists only ciphertext and a wrapping-key identifier for encrypted
secrets. External secrets persist only an allowlisted non-secret reference.
Control reads never return secret values, and manifests contain secret names,
not plaintext or ciphertext.

## Declarative provisioning with `Apply`

Use the bootstrap admin key only long enough to establish normal control and
workload keys. Provision a namespace in this order:

1. `PutSecret` for each encrypted provider credential;
2. `Apply` one manifest containing static provider routes, static agents, and
   operator-owned limits;
3. `IssueServiceKey` for each workload, granting only the necessary
   `consume`/`invoke` operations and explicit agent names;
4. retain each recipient-generated raw key in that recipient's secret manager;
5. issue separate replacement keys for configuration, secret administration,
   key administration, entitlement sync, usage reporting, and audit as the
   deployment requires, then revoke the bootstrap admin key.

`Apply` is transactional for one namespace and is safe under concurrent
repetition. Its idempotency key is bound to the full request semantics; reusing
that key for a different manifest fails. A `dry_run` does not persist an
idempotency record, so the same stable deployment key may be used for the
subsequent real write. Use `expected_revision` when an operator must reject a
stale write. Omitted resources remain untouched unless `prune` is explicitly
enabled; review the dry-run diff before pruning. Prune archives every omitted
current operator limit, agent, and route even when it was already explicitly
disabled. Archived resources are absent from `GetState`; reintroducing the same
agent name through a later manifest allocates a fresh agent identity. Existing
service-key allowlists continue to point at the archived ID and do not gain
authority over the replacement; issue a replacement workload key explicitly.
By contrast, setting `enabled: false` without pruning preserves the agent ID,
so a later re-enable also preserves its allowlists. Archiving an operator limit
releases current ownership of that external key, allowing a later `SyncLimits`
publisher to claim it without deleting historical generations or counter
evidence.

Every route must reference an active **encrypted** secret in the same
namespace. It must declare at least one allowed model, a default model included
in that allowlist, a positive pricing revision, and exactly one non-negative
input/output token price for every allowed model. An omitted price or a price
for a model outside the allowlist rejects the complete `Apply`. Changing price
values requires a strictly higher pricing revision, allowing immutable usage
entries to retain the exact snapshot used for admission and settlement.

Agents are static workload definitions, not tenant copies. A service key with
`consume` or `invoke` must have a non-empty agent allowlist.

The schema can retain an external secret reference, but the built-in V2
provider gateway currently accepts only the `encrypted` backend. It has no
implicit environment/Vault resolver. Provider-specific credential validation
is also not implemented yet: `PutSecret(validate=true)` fails closed instead
of claiming validation. Until a validator lands, write the encrypted secret
with validation disabled and perform a scoped provider smoke test before
activating production traffic.

Application clinics, users, subscriptions, prompts, responses, and PHI do not
belong in an `Apply` manifest. Pass only pseudonymous tenant/actor/billing
references at admission or invocation time.

### Public API bounds

V2 rejects oversized input before storage or provider egress. Public limits
are measured in bytes and are deliberately smaller than some defensive
database column limits:

| Input | Limit |
| --- | --- |
| Opaque refs, including tenant, actor, bill-to, session, feature, model, owner, limit key, and idempotency key | 160 bytes; starts alphanumeric, then ASCII letters/digits plus `.`, `_`, `:`, `/`, `@`, `-` |
| Static names, including application, environment, route, provider, secret, agent, and service-key name | 128 bytes; starts alphanumeric, then path-safe ASCII letters/digits plus `.`, `_`, `-` |
| Metric | 64 bytes; lowercase ASCII metric syntax |
| One `Apply` | 128 routes, 128 agents, and 512 limits |
| Models on one route | 256 allowed-model entries and 256 price entries |
| One `SyncLimits` | 512 limits |
| One service key | 1–8 capabilities and at most 64 allowed agents |
| Encrypted secret plaintext | 64 KiB |
| External secret URI | 2,048 bytes |
| Revocation reason | 256 bytes on one line |
| Reporting | default page 50, maximum page 200, page token 512 bytes, maximum range 366 days |
| Provider JSON request body | 8 MiB |

Tenant and bill-to are mandatory on every admission, provider invocation,
usage query, invocation query, and scoped limit-status request. Optional scope
dimensions must still satisfy the same 160-byte opaque-ref contract.

## Subscription limits with `SyncLimits`

Static, operator-owned limits may live in `Apply`. Dynamic subscription or
entitlement limits should arrive through `SyncLimits` after the application's
own transaction commits, normally from an outbox worker.

Each sync request contains:

- the namespace ID;
- an opaque owner identifying the publishing source (`operator` is reserved);
- a strictly monotonic positive source revision;
- that owner's complete desired limit set;
- a unique idempotency key bound to the request.

Omission disables only limits owned by that source. It cannot remove an
operator limit or another publisher's limit. Repeating the same revision and
content is safe; an older revision, changed content at the same revision, key
reuse with different input, or cross-owner key collision fails closed.

Updating a hard cap, soft cap, or explicit enabled flag keeps the limit's
accounting identity and active counters. A lower cap can therefore take effect
immediately without erasing already consumed or reserved units; subsequent
admission remains blocked until the window resets or the cap again exceeds the
current total. Changing a selector, metric, or window is intentionally a new
accounting identity and creates a fresh immutable generation.

Keep product quota and provider accounting separate. For example,
`ai_actions` can be a clinic/billing-subject monthly product limit, while
`requests`, `input_tokens`, `output_tokens`, and `cost_nano_usd` account for
provider usage. Tenant and billing-subject scopes are mandatory for admission;
actor, session, model, and feature add narrower dimensions. Temporal retries
must reuse the logical operation's idempotency key so one product action is
not counted twice.

Do not call Kave from inside the application's database transaction. Commit
subscription or clinic state first, then publish its materialized limits. A
Kave outage should delay the outbox item, not roll back application onboarding
after a remote side effect has already committed.

## Usage reporting and estimates

`QueryUsage` returns one canonical row per logical product consumption or
provider attempt. Internal per-limit reservation and settlement evidence is
not repeated as billable rows. Provider rows expose request count, input,
output, cache-read, cache-write and reasoning tokens, nano-USD cost,
provider/model, and attempt number. When a metric filter is supplied, `metric`
and `units` project that selected dimension without discarding the complete
provider counters on the row.

Treat `estimated: true` as an accounting provenance marker. It means Kave used
a conservative reservation-derived value because provider usage was missing,
uncertain, smaller than a safely chargeable reservation, or recovered after an
expired attempt whose egress may have started. It is not a provider-reported
exact measurement. `estimated: false` means the row did not require that
fallback; exact product `Consume` rows are not estimates. Never silently mix
estimated and exact totals in billing or operator UI—preserve or surface the
marker.

## Provider attempt recovery

Provider calls hold conservative quota and budget reservations behind renewable
leases. A settlement failure leaves those reservations intact and fail-closed.
After the lease expires, the next provider admission in that namespace first
reconciles a bounded batch of expired attempts: it releases an attempt that
provably never began egress, or charges its reserved maximum when egress may
have begun. The recovery is atomic and namespace-scoped under normal runtime
RLS; it does not require a global maintenance credential or background reaper.

Repeated recovery failures indicate an accounting or database problem. Keep
hard-limit traffic fail-closed, inspect `gateway.recover` audit events and the
corresponding immutable usage entries, and repair database availability rather
than manually editing `limit_windows` or deleting ledger rows.
