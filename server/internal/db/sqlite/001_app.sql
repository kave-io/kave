PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA foreign_keys=ON;
PRAGMA busy_timeout=5000;

-- ── Organizations ──────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS orgs (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL UNIQUE,
    plan       TEXT NOT NULL DEFAULT 'free',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- ── Users ──────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    org_id        TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    email         TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'active',
    last_login_at INTEGER,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL,
    UNIQUE(org_id, email)
);
CREATE INDEX IF NOT EXISTS idx_users_org_id ON users(org_id);

-- ── Memberships ────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS memberships (
    id         TEXT PRIMARY KEY,
    org_id     TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'viewer',
    invited_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at INTEGER NOT NULL,
    UNIQUE(org_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_memberships_org_id ON memberships(org_id);
CREATE INDEX IF NOT EXISTS idx_memberships_user_id ON memberships(user_id);

-- ── Projects ───────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY,
    org_id      TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    UNIQUE(org_id, slug)
);
CREATE INDEX IF NOT EXISTS idx_projects_org_id ON projects(org_id);

-- ── Environments ───────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS environments (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL,
    type       TEXT NOT NULL DEFAULT 'dev',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(project_id, slug)
);
CREATE INDEX IF NOT EXISTS idx_environments_project_id ON environments(project_id);

-- ── Policies ───────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS policies (
    id                 TEXT PRIMARY KEY,
    project_id         TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    env_id             TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    name               TEXT NOT NULL,
    description        TEXT NOT NULL DEFAULT '',
    allowed_types      TEXT NOT NULL DEFAULT '["*"]',
    allowed_connectors TEXT NOT NULL DEFAULT '["*"]',
    allowed_methods    TEXT NOT NULL DEFAULT '["*"]',
    budget_cap_nanos   INTEGER NOT NULL DEFAULT 0,
    budget_period      TEXT NOT NULL DEFAULT 'run',
    budget_behavior    TEXT NOT NULL DEFAULT 'block',
    trace_input        INTEGER NOT NULL DEFAULT 1,
    trace_output       INTEGER NOT NULL DEFAULT 1,
    retention_days     INTEGER NOT NULL DEFAULT 30,
    config             TEXT NOT NULL DEFAULT '{}',
    version            INTEGER NOT NULL DEFAULT 1,
    mode               TEXT NOT NULL DEFAULT 'enforce',
    status             TEXT NOT NULL DEFAULT 'active',
    created_by         TEXT NOT NULL DEFAULT 'system',
    updated_by         TEXT NOT NULL DEFAULT 'system',
    created_at         INTEGER NOT NULL,
    updated_at         INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_policies_project_id ON policies(project_id);
CREATE INDEX IF NOT EXISTS idx_policies_env_id ON policies(env_id);

-- ── Agents ─────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS agents (
    id                   TEXT PRIMARY KEY,
    project_id           TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    env_id               TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    name                 TEXT NOT NULL,
    description          TEXT NOT NULL DEFAULT '',
    policy_id            TEXT REFERENCES policies(id) ON DELETE SET NULL,
    monthly_budget_nanos INTEGER,
    status               TEXT NOT NULL DEFAULT 'active',
    metadata             TEXT NOT NULL DEFAULT '{}',
    created_by           TEXT NOT NULL DEFAULT 'system',
    updated_by           TEXT NOT NULL DEFAULT 'system',
    deleted_at           INTEGER,
    created_at           INTEGER NOT NULL,
    updated_at           INTEGER NOT NULL,
    UNIQUE(env_id, name)
);
CREATE INDEX IF NOT EXISTS idx_agents_project_id ON agents(project_id);
CREATE INDEX IF NOT EXISTS idx_agents_env_id ON agents(env_id);
CREATE INDEX IF NOT EXISTS idx_agents_policy_id ON agents(policy_id);

-- ── Budgets ───────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS budgets (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL UNIQUE REFERENCES agents(id) ON DELETE CASCADE,
    hard_cap_nanos  INTEGER NOT NULL,
    soft_cap_nanos  INTEGER NOT NULL DEFAULT 0,
    period          TEXT NOT NULL DEFAULT 'run',
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_budgets_agent_id ON budgets(agent_id);

-- ── Agent Tokens ───────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS agent_tokens (
    id               TEXT PRIMARY KEY,
    agent_id         TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name             TEXT NOT NULL DEFAULT '',
    description      TEXT NOT NULL DEFAULT '',
    token_prefix     TEXT NOT NULL DEFAULT '',
    hash             TEXT NOT NULL UNIQUE,
    issued_for       TEXT NOT NULL DEFAULT 'agent',
    issued_by        TEXT NOT NULL DEFAULT 'system',
    connectors       TEXT NOT NULL DEFAULT '["*"]',
    methods          TEXT NOT NULL DEFAULT '["*"]',
    budget_cap_nanos INTEGER,
    scopes           TEXT NOT NULL DEFAULT '[]',
    not_before       INTEGER NOT NULL DEFAULT 0,
    expires_at       INTEGER NOT NULL DEFAULT 0,
    last_used_at     INTEGER,
    revoked_at       INTEGER,
    revoked_by       TEXT NOT NULL DEFAULT '',
    revoke_reason    TEXT NOT NULL DEFAULT '',
    created_at       INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_agent_tokens_agent_id ON agent_tokens(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_tokens_hash ON agent_tokens(hash);

-- ── Credentials ────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS credentials (
    id                TEXT PRIMARY KEY,
    project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    env_id            TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    connector_type    TEXT NOT NULL,
    account_id        TEXT NOT NULL DEFAULT '',
    label             TEXT NOT NULL DEFAULT 'primary',
    description       TEXT NOT NULL DEFAULT '',
    source_type       TEXT NOT NULL DEFAULT 'encrypted',
    encrypted_blob    BLOB,
    key_hash          TEXT NOT NULL DEFAULT '',
    wrapping_key_id   TEXT NOT NULL DEFAULT '',
    secret_ref        TEXT NOT NULL DEFAULT '',
    secret_version    TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'active',
    version           INTEGER NOT NULL DEFAULT 1,
    expires_at        INTEGER,
    rotated_at        INTEGER,
    rotated_by        TEXT NOT NULL DEFAULT '',
    last_used_at      INTEGER,
    last_validated_at INTEGER,
    created_by        TEXT NOT NULL DEFAULT 'system',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    revoked_at        INTEGER,
    revoked_by        TEXT NOT NULL DEFAULT '',
    revoke_reason     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_credentials_env_id ON credentials(env_id);
CREATE INDEX IF NOT EXISTS idx_credentials_connector ON credentials(env_id, connector_type);

-- ── Runs ───────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS runs (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    env_id          TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    agent_id        TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    policy_id       TEXT REFERENCES policies(id) ON DELETE SET NULL,
    name            TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'running',
    budget_cap_nanos INTEGER NOT NULL DEFAULT 0,
    spent_nanos     INTEGER NOT NULL DEFAULT 0,
    metadata        TEXT NOT NULL DEFAULT '{}',
    error_message   TEXT,
    trigger_type    TEXT NOT NULL DEFAULT '',
    trigger_id      TEXT,
    correlation_id  TEXT,
    session_id      TEXT,
    idempotency_key TEXT,
    started_at      INTEGER NOT NULL,
    ended_at        INTEGER,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_runs_project_id ON runs(project_id);
CREATE INDEX IF NOT EXISTS idx_runs_env_id ON runs(env_id);
CREATE INDEX IF NOT EXISTS idx_runs_agent_id ON runs(agent_id);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
CREATE INDEX IF NOT EXISTS idx_runs_started_at ON runs(started_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_idempotency ON runs(env_id, idempotency_key) WHERE idempotency_key IS NOT NULL;

-- ── Actions ────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS actions (
    id              TEXT PRIMARY KEY,
    run_id          TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    agent_id        TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    env_id          TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    parent_id       TEXT REFERENCES actions(id) ON DELETE SET NULL,
    action_type     TEXT NOT NULL,
    connector       TEXT NOT NULL,
    method          TEXT NOT NULL,
    input           BLOB,
    output          BLOB,
    error           TEXT,
    started_at      INTEGER,
    ended_at        INTEGER,
    depth           INTEGER NOT NULL DEFAULT 0,
    seq             INTEGER NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'pending',
    source          TEXT NOT NULL DEFAULT 'intercepted',
    metadata        TEXT NOT NULL DEFAULT '{}',
    attempt         INTEGER NOT NULL DEFAULT 1,
    max_attempts    INTEGER NOT NULL DEFAULT 0,
    retry_reason    TEXT,
    provider_req_id TEXT,
    external_id     TEXT,
    created_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_actions_run_id ON actions(run_id);
CREATE INDEX IF NOT EXISTS idx_actions_agent_id ON actions(agent_id);
CREATE INDEX IF NOT EXISTS idx_actions_connector ON actions(connector);

-- ── Budget Ledger ─────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS budget_ledger (
    id                  TEXT PRIMARY KEY,
    project_id          TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    env_id              TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    policy_id           TEXT NOT NULL DEFAULT '',
    agent_id            TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    run_id              TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    action_id           TEXT REFERENCES actions(id) ON DELETE SET NULL,
    span_id             TEXT,
    connector           TEXT NOT NULL,
    model               TEXT NOT NULL DEFAULT '',
    input_tokens        INTEGER NOT NULL DEFAULT 0,
    output_tokens       INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens   INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens  INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens    INTEGER NOT NULL DEFAULT 0,
    audio_input_tokens  INTEGER NOT NULL DEFAULT 0,
    audio_output_tokens INTEGER NOT NULL DEFAULT 0,
    image_units         INTEGER NOT NULL DEFAULT 0,
    request_count       INTEGER NOT NULL DEFAULT 0,
    compute_ms          INTEGER NOT NULL DEFAULT 0,
    storage_bytes       INTEGER NOT NULL DEFAULT 0,
    bandwidth_bytes     INTEGER NOT NULL DEFAULT 0,
    cost_nanos          INTEGER NOT NULL,
    price_version       TEXT NOT NULL DEFAULT '',
    price_snapshot      TEXT,
    usage_detail        TEXT NOT NULL DEFAULT '{}',
    metadata            TEXT NOT NULL DEFAULT '{}',
    created_at          INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ledger_project_id ON budget_ledger(project_id);
CREATE INDEX IF NOT EXISTS idx_ledger_env_id ON budget_ledger(env_id);
CREATE INDEX IF NOT EXISTS idx_ledger_agent_id ON budget_ledger(agent_id);
CREATE INDEX IF NOT EXISTS idx_ledger_run_id ON budget_ledger(run_id);
CREATE INDEX IF NOT EXISTS idx_ledger_created_at ON budget_ledger(created_at DESC);
