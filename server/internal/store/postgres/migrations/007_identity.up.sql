-- sessions
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    user_agent TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT '',
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash);

-- api tokens
CREATE TABLE IF NOT EXISTS api_tokens (
    id TEXT PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash BYTEA NOT NULL UNIQUE,
    scopes TEXT NOT NULL DEFAULT '[]',
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens(token_hash);

-- agent tokens
CREATE TABLE IF NOT EXISTS agent_tokens_new (
    id TEXT PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    name TEXT NOT NULL DEFAULT '',
    token_hash BYTEA NOT NULL UNIQUE,
    scopes TEXT NOT NULL DEFAULT '[]',
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_agent_tokens_new_agent_id ON agent_tokens_new(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_tokens_new_hash ON agent_tokens_new(token_hash);

-- rbac roles
CREATE TABLE IF NOT EXISTS roles (
    id TEXT PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    permissions TEXT NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_roles_org_id ON roles(org_id);

-- rbac bindings
CREATE TABLE IF NOT EXISTS bindings (
    id TEXT PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    subject TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT '*',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bindings_org_id ON bindings(org_id);
CREATE INDEX IF NOT EXISTS idx_bindings_role_id ON bindings(role_id);
