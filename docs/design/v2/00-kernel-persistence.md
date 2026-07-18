# Kave V2 kernel persistence contract

Status: accepted foundation, implemented side-by-side with V1

Scope: `server/internal/v2/postgres`

## Outcome

Kave V2 persistence is a small, Postgres-only tenant-isolation and accounting
kernel. It does not reuse the V1 aggregate `AppStore`, migrations, tables, or
runtime lifecycle RPCs. It is wired side-by-side on the existing HTTP listener
when `v2.enabled` is true; V1 remains available during migration. The
authenticated Connect surface implements transactional `Apply`/`GetState`,
service-key and secret administration, `SyncLimits`, exact `Consume`, scoped
limit status, usage/invocation reporting, and audit reads. The same listener
also exposes the authenticated OpenAI-compatible V2 gateway for chat
completions, Responses, and embeddings.

The kernel owns ten domain tables:

1. `namespaces`
2. `secrets`
3. `provider_routes`
4. `agents`
5. `service_keys`
6. `limits`
7. `limit_windows`
8. `invocations`
9. `usage_entries`
10. `audit_events`

`schema_migrations` is operational metadata and is not a domain table.

## Boundaries

The V2 kernel does:

- isolate account/application/environment namespaces;
- persist static AI workload definitions and provider routes;
- store provider-secret ciphertext or external secret references;
- authenticate namespace-scoped service keys;
- materialize fixed-scope quotas and budgets;
- provide atomic counters for admission reservations and consumption;
- record logical invocations, provider attempts, usage, and audit evidence.

The V2 kernel does not:

- own Simorq clinics, users, plans, subscriptions, or PHI;
- persist prompts, responses, retrieved documents, or tool payloads;
- run agents or schedule provider calls;
- provide human users, passwords, sessions, memberships, roles, or Casbin;
- provide SQLite, DuckDB, or ClickHouse implementations;
- automatically migrate ambiguous V1 workspace/agent records;
- expose secret values through read models;
- initiate FX or pricing network refreshes.

Kave Cloud may own human identity and billing above this kernel. An application
such as Simorq remains authoritative for membership, entitlements, and product
semantics and publishes only pseudonymous scopes and materialized limits.

## Ten-table model

### Namespace

`namespaces` is the tenancy root:

```text
account_id + application + environment
```

The tuple is unique. `account_id` is supplied by the trusted control plane; a
self-hosted installation may use a configured local account ID. Namespace IDs
are known before insertion, allowing a transaction to install its desired RLS
scope before creating the namespace row.

There is deliberately no organizations/projects dual-use table.

### Static configuration

`provider_routes` contains the upstream protocol, URL, secret reference, model
policy, and versioned pricing configuration. Although the defensive schema can
represent a nullable or external secret reference, declarative route activation
requires an active encrypted secret. A route also requires a non-empty model
allowlist, a default in that allowlist, a positive pricing revision, and one
input/output token price for every allowed model. Each usage entry stores the
exact price snapshot used, so changing a route never rewrites historical
accounting.

`agents` is a small set of static workload definitions such as
`clinic-assistant`, `public-docs-assistant`, and `clinical-indexer`. A tenant is
a request scope, not an agent copy. Disabling and re-enabling an agent preserves
its ID. Pruning archives it; later reintroduction of the same name allocates a
fresh ID, so service keys allowlisted to the archived ID do not implicitly gain
access to the replacement.

`service_keys` contains machine identities. A key is restricted to one
namespace, explicit capabilities, and explicit agent IDs. Empty allowed-agent
sets grant no invocation access. The recipient/client generates raw key
material before issuance and sends only a 32-byte SHA-256 verifier plus a
non-secret lookup prefix; the serving issuance API never receives or returns
the raw credential. Offline bootstrap performs the same client step locally
before database mutation. The server therefore cannot persist or recover the
credential. The canonical key contains a 256-bit secret and an independent
144-bit lookup prefix encoded as exactly 24 unpadded base64url characters. It
is later presented only as an authentication bearer. Both the idempotency hash
and natural-key replay check bind the supplied prefix and verifier. A retry
with the same material can return metadata safely; different material
conflicts instead of silently rotating a key.

Persisted authority is split across eight operations rather than a broad
administrator flag: `config.apply`, `secrets.write`, `keys.manage`,
`limits.sync`, `usage.read`, `audit.read`, `consume`, and `invoke`.
`config.apply` covers only manifests and `GetState`. Workload operations require
explicit agent IDs and scope-assertion permission; control, reporting, and audit
keys can be separated without also granting provider invocation.

### Secrets

`secrets.backend` has exactly two values:

- `encrypted`: ciphertext plus an explicit wrapping-key/KMS identifier;
- `external`: a non-secret URI understood by an allowlisted resolver.

The row check requires exactly one representation. Vault and supported cloud
secret-manager references use external URIs rather than competing database
enums; environment and passthrough credentials are not production storage
modes. The built-in provider gateway currently has no external resolver, so an
active route must use the encrypted backend. A server without an explicit
master key/KMS configuration must refuse encrypted-secret writes. Read APIs
return metadata only.

### Limits and counters

`limits` holds one metric and hard cap, an optional soft cap, and nullable exact
selectors:

```text
tenant_ref actor_ref billing_ref agent_id model feature
```

`NULL` means wildcard. A limit with no selectors is namespace-wide. Combining
selectors supports a clinic's `ai_actions`, a user's token budget, a billing
subject's cost cap, or an agent/model constraint without adding a Kave user
record. Public opaque refs are pseudonymous ASCII values capped at 160 bytes;
the wider 255-byte database checks are defense in depth, not the API contract.

The public V2 `LimitSpec` supports calendar day, calendar month, and lifetime
(`all_time`) windows. The persistence schema reserves fixed-duration and
explicit-interval shapes for a future contract; current clients cannot publish
them.

`limit_windows` is the only mutable accounting counter. Each row contains
`used` and `reserved`, both non-negative. Admission creates and locks applicable
rows in deterministic limit-ID order. Exact consumption uses a short
`READ COMMITTED` transaction plus explicit namespace and counter row locks;
this avoids serializable-abort starvation on hot clinic counters without
weakening the atomicity boundary.

Hard-cap, soft-cap, and explicit enablement changes are policy updates on the
same limit ID. They advance the limit revision but retain every active
`limit_windows` row, including outstanding reservations that a provider
attempt will later settle. Selector, metric, or window changes define a new
accounting identity; those changes archive the old immutable generation and
start a new one. Apply and `SyncLimits` hold the namespace write lock across
this choice, so admission observes one complete policy and cannot lose usage
at the reconciliation boundary.

### Invocation and immutable ledgers

`invocations` represents one logical operation, either an exact product-quota
consume or a provider call. It stores pseudonymous scope, topology, status, a
request hash, and an idempotency key. It never stores request or response
bodies.

The unique tuple is:

```text
account + namespace + operation + idempotency key
```

Idempotency therefore survives service-key rotation. Reusing that tuple with a
different request hash is a conflict. Exact `Consume` returns its original
decision. A successful provider invocation cannot replay a response body (Kave
does not persist one) and returns a conflict; an unsuccessful provider
invocation may allocate another attempt beneath the same logical invocation.

The original admission result is stored in the bounded `decision` JSON object
(maximum 16 KiB), including violations and warnings. A replay returns this
snapshot and never reevaluates the request against limits that may have changed
since the first decision.

`usage_entries` is append-only. Event kinds distinguish reservation, release,
consume, provider attempt, settlement, adjustment, and block evidence.
Limit-associated entries retain the exact counter window. Provider usage uses
integer request/token/unit fields, integer money, currency, provider/model,
attempt number, and an immutable pricing snapshot. Public `QueryUsage` returns
only canonical logical-consume/provider-attempt rows rather than internal
per-limit duplicates. Its `estimated` marker is true when conservative
reservation or recovery values replaced missing or uncertain provider-reported
usage; consumers must retain that provenance when aggregating totals.

`audit_events` is append-only control/security evidence. Its `details` object
must be redacted before insertion and must never contain secrets or AI payloads.

Database triggers reject update and delete operations on both immutable tables.
Corrections use compensating entries rather than mutation.

## Isolation contract

Every domain table is account-owned. Every table below `namespaces` duplicates
both `account_id` and `namespace_id`.

Parents expose composite unique keys and children use composite foreign keys.
Consequently, a row cannot reference a secret, route, agent, service key,
limit, or invocation from another account/namespace even if an application
query is defective.

All ten tables have both:

```sql
ENABLE ROW LEVEL SECURITY
FORCE ROW LEVEL SECURITY
```

The namespace policy matches `kave.account_id`. All child policies match both
`kave.account_id` and `kave.namespace_id`. Missing settings yield no rows, not
an unscoped view.

The runtime database role must:

- not be a superuser;
- not have `BYPASSRLS`;
- not own the V2 schema, tables, or pre-authentication function;
- not be a member of the migration-owner role;
- connect directly as the runtime login role rather than assuming it with
  `SET ROLE` from a privileged session;
- receive only the table/function grants required by the kernel.

Migrations must run under a distinct, non-login owner role. Service-key
authentication calls the narrowly defined `SECURITY DEFINER`
`kave_v2.lookup_service_key(prefix)` function, which is the only pre-RLS
identity lookup permitted. The function:

- pins `search_path` to `pg_catalog, pg_temp` and fully qualifies its table;
- keeps `row_security` enabled and opens an RLS policy only for the exact prefix
  while `current_user` is the definer and differs from `session_user`;
- returns only account ID, namespace ID, key ID, verifier, capabilities,
  allowed agents, scope permission, status, and expiry;
- receives a non-secret prefix, never the complete raw key;
- has all `PUBLIC` privileges revoked.

Deployment explicitly grants the runtime role `USAGE` on the schema and
`EXECUTE` on this function, but no unscoped `SELECT` on `service_keys`. The Go
authenticator hashes and constant-time compares the raw key in process, rejects
missing/revoked/expired keys with one credential error, and only then installs
the returned account/namespace scope through `ScopedRunner`.

## Scoped transaction protocol

Application queries enter through `ScopedRunner.WithScope`. It:

1. validates non-empty account and namespace IDs;
2. begins a `SERIALIZABLE` transaction for control operations;
3. installs identity using:

   ```sql
   set_config('kave.account_id', ..., true)
   set_config('kave.namespace_id', ..., true)
   ```

4. invokes the callback with query methods but no commit/rollback methods;
5. commits on success and rolls back on any error.

The third `set_config` argument must remain `true`. Connection-level settings
are forbidden because a pooled connection could leak one tenant into the next
request. Long-lived streaming RPCs must not hold a transaction; each database
read opens a short scoped transaction.

Account-scoped offline bootstrap `Apply` begins with the deterministic
namespace-ID candidate. To support an existing namespace whose ID predates
that derivation, bootstrap additionally installs the manifest's exact
application/environment tuple for the initial lookup. The namespace RLS policy
exposes only that tuple within the authenticated account, while its `WITH
CHECK` remains bound to the candidate ID. After an active row is resolved,
bootstrap replaces the namespace scope with the stored ID and immediately
clears the tuple settings. A disabled matching namespace is a kill switch and
is never recreated or reactivated under the deterministic ID. Ordinary
`config.apply` service keys start at their authenticated namespace ID and must
match both that exact ID and the manifest tuple; they never enable tuple lookup
or namespace discovery.

The transaction helper itself does not retry callbacks because arbitrary
callbacks are not necessarily safe to repeat. Apply and service-key operations
own bounded retries. Exact admission uses the same scope installation with
`READ COMMITTED`, locks the namespace `FOR SHARE`, and locks matching window
rows in stable order. Apply and `SyncLimits` take the namespace
`FOR UPDATE`, so admission observes either the old or new complete limit set.

## Atomic admission contract

Exact product-quota `Consume` uses these tables in this order:

1. Insert or lock the idempotent invocation.
2. Resolve every matching enabled limit.
3. Create missing window rows and lock them in stable limit-ID order.
4. Check every hard cap using `used + reserved + delta` with checked integers.
5. Either update all counters or none.
6. Commit rejected invocation/audit evidence without consuming counters.
7. For exact product units, increment `used` immediately.
8. Return the original immutable decision for a same-input replay and reject
   reuse of the key with different input.

Provider gateway reservation and settlement implements the same invariant. It
increments `reserved` conservatively, commits, executes provider traffic
outside the transaction, then atomically converts the reservation to actual
usage and appends immutable attempt evidence. Matching request, input-token,
output-token, and nano-USD limits are all evaluated. Token/cost limits fail
closed unless the request supplies safe bounds and the route supplies a price
for the selected model.

The HTTP surface is intentionally exact:

```text
POST /v2/agents/{agent}/openai/chat/completions
POST /v2/agents/{agent}/openai/responses
POST /v2/agents/{agent}/openai/embeddings
```

It requires an invoke-capable service key, an explicit resolved-agent
allowlist, `Kave-Tenant`, `Kave-Bill-To`, and a stable
`X-Kave-Invocation-Key`. Optional actor/session/feature values are opaque
pseudonymous refs. Query parameters and caller/provider credentials are not
forwarded. Only allowlisted request headers leave Kave; the decrypted provider
credential is injected inside the process and cleared after the request.
The purpose-built provider transport ignores environment proxies and validates
every resolved address before each dial. Private, loopback, link-local,
multicast, unspecified, reserved, and metadata destinations fail closed by
default; a local/self-hosted exception is an operator-supplied list of exact
private or loopback IP literals. DNS names and CIDRs are not exception keys.
Kave dials only the validated resolved IP while retaining the original Host and
TLS server name, preventing a validation/dial DNS-rebinding gap.

Each admitted provider attempt has a renewable lease. A same-key retry while
the lease is active returns an in-progress conflict. If a worker crashed before
the durable attempt-start entry, an expired reservation is released; if egress
may have started, it is charged at the conservative maximum before attempt N+1
can reserve. Before every provider admission, Kave reconciles a bounded batch
of expired attempts in the authenticated namespace. Rows are claimed with
`FOR UPDATE SKIP LOCKED`, so concurrent processes cannot settle an orphan twice
and a different invocation key can make progress after a failed settlement.
This remains RLS-scoped and needs no cross-tenant privileged reaper or scheduler.

Hard-limit accounting errors fail closed. Actual provider usage is never
dropped, even when it exceeds an estimate. Recovery, counter conversion,
immutable usage evidence, and the terminal invocation state commit atomically.

## Migration contract

V2 migrations are embedded only from:

```text
server/internal/v2/postgres/migrations/*.up.sql
```

They use a separate `kave_v2.schema_migrations` registry. File names have a
three-digit, unique, monotonic version. The migrator:

- validates names and duplicate versions before touching the database;
- computes SHA-256 checksums from embedded bytes;
- acquires a transaction-scoped advisory lock;
- executes the migration and registry insert in the same transaction;
- rejects changed names or checksums for an applied version.

Applied migrations are immutable. Never renumber or edit one; add the next
version. Functions and triggers use ordinary `CREATE`, not `OR REPLACE` or
ad-hoc existence guards; migration history is the only idempotency mechanism.
V1 migration history is neither read nor changed.

Serving and migration roles are deliberately separate. The terminating
`v2-migrate` command receives `KAVE_V2_MIGRATION_DSN`, the non-login owner, and
the runtime-role name; the serving process never reads migration authority.
The migration connection applies migrations, rejects a runtime role that is
superuser, `BYPASSRLS`, or inherits the migration owner, and grants only
required DML plus the prefix lookup function. The grant step revokes stale
direct and `PUBLIC` privileges and installs restrictive owner default
privileges, making repeated migration runs convergent. Serving then opens a
separate pool that logs in directly as `v2.runtime_role` and verifies its exact
effective relation and function grants. Any privilege on a future table, view,
sequence, or function fails startup until an explicit kernel grant change is
shipped.

## Required tests

The package must keep fast structural/unit tests for:

- exactly ten domain tables;
- RLS enabled and forced on every table;
- account/namespace policies and composite foreign keys;
- fixed scope columns and idempotency uniqueness;
- bounded, replayable invocation decisions;
- prefix-only pre-authentication and constant-time raw-key verification;
- immutable usage/audit triggers;
- absence of prompt/response body columns;
- migration ordering, checksums, advisory locking, and drift rejection;
- serializable control transactions, locked read-committed admission, and
  transaction-local scope installation;
- rollback on callback failure and validation before transaction begin.

Opt-in real-Postgres tests additionally verify repeatable migration, scope
visibility, atomic concurrent caps, Apply replay/prune behavior, service-key
authentication/revocation, envelope-encrypted secret rotation, and a full
offline bootstrap → authenticated Apply → key issue → Consume flow using a
distinct non-owner, non-`BYPASSRLS` runtime role. The required CI job supplies
`KAVE_TEST_V2_POSTGRES_DSN` with separate fresh store and kernel databases, and
`KAVE_TEST_V2_POSTGRES_ADMIN_DSN` only to create an isolated database plus
short-lived owner/runtime roles for adversarial privilege tests. These tests
run in mandatory CI with the race detector; the V2 packages are also vetted.

## Completion criteria for this foundation

- V1 remains available while V2 registration is explicitly gated.
- The V2 package and transport-neutral core compile independently.
- The embedded migration is deterministic and checksum guarded.
- No callback can run before RLS identity is installed.
- A query with missing or wrong scope cannot see account-owned rows.
- Database constraints reject cross-account references.
- Usage and audit evidence cannot be rewritten or deleted.
- No schema column invites storage of AI request/response bodies.
