PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA foreign_keys=ON;
PRAGMA busy_timeout=5000;

CREATE TABLE IF NOT EXISTS workspaces (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_workspaces_slug ON workspaces(slug);

CREATE TABLE IF NOT EXISTS policies (
    id                  TEXT PRIMARY KEY,
    workspace_id        TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    description         TEXT,
    allowed_connectors  TEXT NOT NULL DEFAULT '["*"]',
    allowed_methods     TEXT NOT NULL DEFAULT '["*"]',
    budget_cap_usd      REAL NOT NULL DEFAULT 1000.0,
    config              TEXT NOT NULL DEFAULT '{}',
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_policies_workspace_id ON policies(workspace_id);

CREATE TABLE IF NOT EXISTS agents (
    id              TEXT PRIMARY KEY,
    workspace_id    TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT,
    policy_id       TEXT REFERENCES policies(id) ON DELETE SET NULL,
    monthly_budget  REAL NOT NULL DEFAULT 100.0,
    metadata        TEXT NOT NULL DEFAULT '{}',
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    UNIQUE(workspace_id, name)
);
CREATE INDEX IF NOT EXISTS idx_agents_workspace_id ON agents(workspace_id);
CREATE INDEX IF NOT EXISTS idx_agents_policy_id ON agents(policy_id);

CREATE TABLE IF NOT EXISTS runs (
    id              TEXT PRIMARY KEY,
    workspace_id    TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id        TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    policy_id       TEXT REFERENCES policies(id) ON DELETE SET NULL,
    name            TEXT,
    status          TEXT NOT NULL DEFAULT 'active',
    budget_cap_usd  REAL,
    spent_usd       REAL NOT NULL DEFAULT 0.0,
    metadata        TEXT NOT NULL DEFAULT '{}',
    error_message   TEXT,
    started_at      INTEGER NOT NULL,
    ended_at        INTEGER,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_runs_workspace_id ON runs(workspace_id);
CREATE INDEX IF NOT EXISTS idx_runs_agent_id ON runs(agent_id);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
CREATE INDEX IF NOT EXISTS idx_runs_started_at ON runs(started_at DESC);

CREATE TABLE IF NOT EXISTS actions (
    id          TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    action_type TEXT NOT NULL,
    connector   TEXT NOT NULL,
    method      TEXT NOT NULL,
    input       TEXT NOT NULL,
    metadata    TEXT NOT NULL DEFAULT '{}',
    created_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_actions_run_id ON actions(run_id);
CREATE INDEX IF NOT EXISTS idx_actions_type ON actions(action_type);
CREATE INDEX IF NOT EXISTS idx_actions_connector ON actions(connector);

CREATE TABLE IF NOT EXISTS budget_ledger (
    id                  TEXT PRIMARY KEY,
    workspace_id        TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id            TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    run_id              TEXT REFERENCES runs(id) ON DELETE SET NULL,
    action_id           TEXT REFERENCES actions(id) ON DELETE SET NULL,
    span_id             TEXT,
    connector           TEXT NOT NULL,
    model               TEXT,
    input_tokens        INTEGER,
    output_tokens       INTEGER,
    cache_read_tokens   INTEGER,
    cache_write_tokens  INTEGER,
    cost_usd            REAL NOT NULL,
    metadata            TEXT NOT NULL DEFAULT '{}',
    created_at          INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_budget_ledger_workspace_id ON budget_ledger(workspace_id);
CREATE INDEX IF NOT EXISTS idx_budget_ledger_agent_id ON budget_ledger(agent_id);
CREATE INDEX IF NOT EXISTS idx_budget_ledger_run_id ON budget_ledger(run_id);
CREATE INDEX IF NOT EXISTS idx_budget_ledger_action_id ON budget_ledger(action_id);
CREATE INDEX IF NOT EXISTS idx_budget_ledger_created_at ON budget_ledger(created_at DESC);

CREATE TABLE IF NOT EXISTS credentials (
    id              TEXT PRIMARY KEY,
    workspace_id    TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    connector       TEXT NOT NULL,
    label           TEXT NOT NULL,
    key_hash        TEXT NOT NULL,
    encrypted       BLOB NOT NULL,
    last_used_at    INTEGER,
    created_at      INTEGER NOT NULL,
    UNIQUE(workspace_id, connector, key_hash)
);
CREATE INDEX IF NOT EXISTS idx_credentials_workspace ON credentials(workspace_id);
CREATE INDEX IF NOT EXISTS idx_credentials_connector ON credentials(workspace_id, connector);

CREATE TABLE IF NOT EXISTS agent_tokens (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    connectors      TEXT NOT NULL DEFAULT '["*"]',
    methods         TEXT NOT NULL DEFAULT '["*"]',
    budget_cap_usd  REAL,
    expires_at      INTEGER NOT NULL,
    created_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_agent_tokens_agent_id ON agent_tokens(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_tokens_expires_at ON agent_tokens(expires_at);

CREATE TABLE IF NOT EXISTS revoked_tokens (
    id              TEXT PRIMARY KEY,
    token_id        TEXT NOT NULL UNIQUE,
    revoked_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_revoked_tokens_token_id ON revoked_tokens(token_id);

CREATE TABLE IF NOT EXISTS schema_migrations (
    name        TEXT PRIMARY KEY,
    applied_at  INTEGER NOT NULL
);
