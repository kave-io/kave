# Plan 14 — Kave Cloud: Hosted SaaS Layer (Separate Repo)

**Goal:** design, not implement. Kave-cloud lives in `github.com/kave-io/kave-cloud`, consumes `github.com/kave-io/kave` as a library where practical, and extends it with multi-tenancy, billing, team features, and managed infra. Free tier is "kave local in the cloud" for solo devs (2 projects max). Paid tiers add teams, SSO, audit retention, advanced cost tools, SOC 2 posture.

This plan is the contract the cloud repo will implement. Nothing here ships in the OSS repo except minor seams (noted explicitly in §9).

## Product tiers

| Tier | Price | Limits |
|---|---|---|
| **Free** | $0 | 1 org, 1 user, 2 projects, 3 envs/project, 14-day span retention, 50k spans/mo, community support |
| **Team** | $29/seat/mo | Unlimited projects/envs, 90-day retention, RBAC roles, Slack alerts, per-team budgets |
| **Enterprise** | custom | SSO/OIDC, SOC 2 reports, custom retention, VPC peering, audit-log export, SLA, dedicated support |

Every tier is the same binary (kave-cloud control plane). Tier differences are feature flags + limits in the billing/entitlements service.

## Architecture

```
                     ┌─────────────────────────────┐
Users  ───HTTPS──▶  │ api.kave.io (edge)          │
                    │ - CF / Cloud LB              │
                    │ - OIDC / PASETO auth         │
                    └──────┬──────────────────────┘
                           │ gRPC (mTLS)
                    ┌──────▼──────────────────────┐
                    │  Control Plane (kave-cloud) │
                    │  - multi-tenant gRPC+HTTP   │
                    │  - uses kave/core as lib    │
                    │  - adds Billing, SSO, Orgs  │
                    └──────┬──────────────────────┘
                           │
                    ┌──────▼──────────────────────┐
                    │  Data (multi-tenant)        │
                    │  - Postgres (row-level org) │
                    │  - ClickHouse (spans, hot)  │
                    │  - S3 + Parquet (spans cold)│
                    │  - Vault (tenant secrets)   │
                    └─────────────────────────────┘

                    ┌─────────────────────────────┐
Agents ──HTTPS──▶  │ proxy.kave.io (edge)        │
                    │  - ingest LLM calls          │
                    │  - routes to tenant daemon   │
                    │  - or dataplane-shared       │
                    └─────────────────────────────┘
```

### Shared-DB multi-tenancy (v1 cloud)

All tenant data lives in the same Postgres cluster, partitioned by `org_id` via Postgres Row-Level Security. Kave's core already carries `OrgID` on every row — this is the key architectural win.

```sql
ALTER TABLE agents ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_iso ON agents USING (org_id = current_setting('kave.org_id')::text);
```

Every request sets `SET LOCAL kave.org_id = '<org>'` from the session's identity. One bug = cross-tenant leak, so:
- Wrap pool acquisition: `pool.Acquire(ctx).Tenant(orgID)`.
- Integration tests: for every endpoint, make request as Org A, try to read Org B's data → assert 404.

DB-per-org is deferred to Enterprise (higher isolation, at which point we replicate the schema and route by org_id at the connection level).

### Span storage (hot/cold)

- Hot: ClickHouse cluster, TTL = tier retention window (14d/90d/custom).
- Cold: S3 Parquet, partitioned by `org_id/year/month/day`. `SELECT * FROM spans` older than hot TTL goes through a federation layer.
- Why ClickHouse over DuckDB/Postgres: write volume expectation is 10k–100k spans/sec across tenants. CH is the only open-source column store that handles this at our expected cost.

The OSS `SpanStore` interface already supports this — cloud implements a `ClickHouseSpanStore` satisfying the same contract. No changes needed in `core/store/`.

## Data model additions (cloud-only tables)

Extend the OSS core model. Cloud adds tables alongside:

```go
// Tenant (== Organization + plan + billing)
type CloudOrg struct {
    ID            string         // org_<ulid> (reuses OSS org)
    Plan          string         // "free" | "team" | "enterprise"
    TrialEndsAt   *time.Time
    StripeID      string         // Stripe customer ID
    Entitlements  Entitlements   // feature flags derived from plan + overrides
    CreatedAt     time.Time
    SuspendedAt   *time.Time     // dunning, tos violation, etc.
}

type Entitlements struct {
    MaxProjects        int
    MaxEnvsPerProject  int
    MaxUsers           int
    SpanRetentionDays  int
    MonthlySpanQuota   int64
    SSO                bool
    AuditExport        bool
    SlackAlerts        bool
    CustomRetention    bool
    VPCPeering         bool
}

type Subscription struct {
    ID            string         // sub_<ulid>
    OrgID         string
    StripeSubID   string
    Status        string         // active, past_due, canceled
    CurrentPeriod DateRange
    SeatCount     int
    PlanID        string
}

type Invoice struct {
    ID            string
    OrgID         string
    StripeInvoice string
    Amount        money.Amount
    Currency      string        // Stripe source of truth; we cache
    Status        string        // draft, open, paid, void
    PeriodStart   time.Time
    PeriodEnd     time.Time
}

type UsageRecord struct {
    // for metering tiers (spans/mo, seats, API calls)
    ID        string
    OrgID     string
    Metric    string            // "spans.ingested", "seats.active", "gateway.requests"
    Quantity  int64
    RecordedAt time.Time        // minute bucket
}

// SSO
type IdentityProvider struct {
    ID       string
    OrgID    string
    Kind     string             // "oidc", "saml"
    Config   json.RawMessage    // per-kind blob, encrypted at rest
    Status   string             // active, disabled
}

// Cross-org invitations
type Invitation struct {
    ID        string            // inv_<ulid>
    OrgID     string
    Email     string
    Role      string            // admin/dev/viewer/billing (OSS roles)
    Token     string            // hashed; raw sent by email
    InvitedBy string            // user_id
    ExpiresAt time.Time
    AcceptedAt *time.Time
}

// Alerts
type AlertRule struct {
    ID        string
    OrgID     string
    ProjectID string            // optional
    Kind      string            // "budget.threshold", "policy.block_rate", "error.spike"
    Config    json.RawMessage
    Targets   []AlertTarget     // slack, email, webhook
    Enabled   bool
}

type AlertTarget struct {
    Kind   string               // "slack", "email", "webhook"
    Value  string               // webhook URL, channel id, email
    Secret string               // encrypted
}

// Audit export (Enterprise)
type AuditExportJob struct {
    ID        string
    OrgID     string
    Format    string            // "jsonl", "parquet"
    Range     DateRange
    Status    string            // queued, running, ready, failed
    S3Key     string
    URL       string            // signed URL for download (TTL 24h)
    CreatedAt time.Time
}

// Tenant-scoped encryption keys
type TenantKey struct {
    ID        string            // tkey_<ulid>
    OrgID     string
    Version   int
    KMSKeyARN string            // each tenant gets their own KMS key for their secrets
    State     string            // active, rotating, retired
    CreatedAt time.Time
}

// Dataplane assignment (for isolated dataplanes on Enterprise)
type DataplaneAssignment struct {
    OrgID       string
    Region      string
    Cluster     string            // "shared-us" or "dedicated-<org>-us"
    IngestURL   string
    ControlURL  string
}
```

## Services (gRPC)

Cloud repo adds services alongside the OSS control-plane/runtime services:

```
proto/kavecloud/v1/
├── billing.proto       — BillingAPI: subscriptions, invoices, stripe webhooks
├── identity.proto      — IdentityAPI: OIDC/SAML CRUD, invitations, SSO login
├── entitlements.proto  — EntitlementsAPI: Get, Check (used by edge gateway)
├── alerts.proto        — AlertsAPI: rule CRUD, test, delivery log
├── usage.proto         — UsageAPI: current-period metering, history
├── audit_export.proto  — AuditExportAPI: request export, download
└── cloud.proto         — CloudAPI: roll-up "tenant" ops (suspend, wipe, export-all)
```

Edge auth flow:
1. User logs in (OIDC or password).
2. Edge issues PASETO `{sub: usr_..., org: org_..., plan: team}`.
3. Every request carries that PASETO. Edge verifies + injects `org_id` into the DB session before any query.

## Billing flow

Stripe as the billing system. Cloud implements:

1. `POST /v1/billing/checkout` — creates Stripe Checkout session for plan upgrade.
2. `POST /v1/billing/webhook` — Stripe webhook listener. Events: `invoice.paid`, `customer.subscription.updated`, `customer.subscription.deleted`. Updates `Subscription`, recomputes `Entitlements`, caches decision.
3. Usage-based billing on Team+: nightly job aggregates `UsageRecord` → pushes `Stripe.UsageRecord.Create` for metered items (spans over quota, additional seats).
4. Dunning: `past_due` → email at +1d, +3d, +7d; suspend at +14d; delete at +45d.
5. Free tier: no Stripe customer until upgrade. Limits enforced by `Entitlements` alone.

## Enforcement surface

`Entitlements` is the single source of truth for limits. Every write endpoint calls `entitlements.Check(orgID, "projects.create")` which returns `allow / deny / upgrade_required`. Deny → 403 with an `upgrade_url` in the error envelope. The UI renders these as upgrade CTAs.

Rate-limit table (per plan, per endpoint class):

| Endpoint class | Free | Team | Enterprise |
|---|---|---|---|
| Gateway (LLM proxy) | 60 req/min | 1200 req/min | custom |
| Control plane CRUD | 30 req/min | 300 req/min | custom |
| Streams (SSE/gRPC stream) | 5 concurrent | 50 concurrent | custom |

Enforce via Redis token-bucket in the edge.

## Secrets

Per-tenant DEKs, wrapped by AWS KMS (one CMK per tenant on Enterprise, one shared CMK + per-tenant DEK on Free/Team). Agent credentials encrypted with tenant DEK before insert. Keyring abstraction already exists in `core/pkg/keyring` — cloud implements a `KMSKeyring` against it.

Rotation: on customer request or quarterly. `TenantKey` versioning supports re-encrypt workflows.

## Observability

- Cloud daemon emits OTLP spans to Honeycomb (or equivalent). Don't eat our own OTel dogfood directly — customer spans are their data, ours are ours.
- Per-tenant cost ledger: store `sum(cost)` by `(org_id, day)` in a rollup table. Used for billing + user-facing cost view.
- Health probes: `/_/health` (deep: DB, CH, Stripe), `/_/ready` (shallow).

## Migration path from self-host

One-click import: `kave cloud import --from ./kave.db ./kave-spans.duckdb`. Creates org, uploads records, sets retention. Keep local copy untouched.

## Separate-repo consumption

`kave-cloud/go.mod`:
```
require (
    github.com/kave-io/kave/core v1.0.0
    github.com/kave-io/kave/proto/gen v1.0.0
)
```

Only `core/` and `proto/gen/` are stable enough to depend on. `server/` internals are cloud's to fork and adapt — cloud copies the gateway/httpbridge pattern rather than depending on it, because cloud's versions are multi-tenant and diverge fast.

## OSS-side seams this plan requires (do these in THIS repo)

These are the only OSS changes kave-cloud needs; list here so they land before cloud work:

1. `core/store.OrgScope(ctx)` helper that stores `org_id` on ctx and is required for all store-level calls. Non-breaking for single-tenant (empty = "default org").
2. `core/store.SpanStore` already abstract — verify the postgres and duckdb implementations honor `OrgID` filters without bolt-on query rewrites.
3. `core/pkg/keyring.Backend` interface so cloud can drop in a KMS-backed implementation.
4. `core/pkg/entitlements` (optional stub): a no-op `Check` that always allows; cloud replaces with real impl. OSS ships with "always allow" and the entitlements struct shape so contracts match.
5. Publish `core/` and `proto/gen/` as separate module versions; kave-cloud pins them.

## Open decisions (resolve before starting cloud repo)

- **Region strategy v1**: us-east only, or us-east + eu-west on launch? EU requires data-residency story for GDPR; delay unless we have a committed EU customer.
- **Dedicated dataplane**: Enterprise gets a dedicated ingest URL pointing at their own CH cluster; which region/AZ setup? (Decision: us-east-2, multi-AZ. EU customers wait for eu-west-1.)
- **Team tier price anchor**: $29/seat/mo is standard for devtools but may be aggressive for a middleware category. Validate with 5 target customers before shipping pricing.
- **Audit retention on Team**: 90 days is generous but storage cost is material. Reconsider after 2 months of real usage data.
- **Ollama/local model support on cloud**: do we proxy to a customer-hosted Ollama over reverse tunnel, or disallow? Decision: disallow on Free/Team; Enterprise gets a sidecar agent that relays.

## Timeline

- **Weeks 0–2**: OSS seams (§9), stabilize v1 in this repo.
- **Weeks 2–4**: cloud repo skeleton, multi-tenant Postgres + RLS, edge auth, entitlements.
- **Weeks 4–6**: Stripe integration, checkout, webhook, free→team upgrade flow.
- **Weeks 6–8**: ClickHouse span ingest, cold-tier S3 Parquet, retention jobs.
- **Weeks 8–10**: alerts, invitations, SSO (OIDC-only first).
- **Weeks 10–12**: perf + closed beta with 10 design partners.
- **Week 12+**: general availability once SLOs hold for 2 weeks.

## Non-goals for v1 cloud

- SAML SSO (OIDC first; SAML on demand).
- On-prem installer beyond existing OSS.
- Marketplace (connectors, policies) — depends on community flywheel.
- Mobile app.
- Anything touching training data or fine-tuning.

## Size estimate
Not applicable (cross-repo, multi-month). For this OSS-side prep: ~200 LOC of seams from §9 and publish step. One haiku session for that scope only.
