-- revoked_tokens — tracks revoked agent tokens
CREATE TABLE revoked_tokens (
    id BIGSERIAL PRIMARY KEY,
    token_id TEXT NOT NULL UNIQUE,
    revoked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_revoked_tokens_token_id ON revoked_tokens(token_id);

-- agent_tokens — active agent authorization tokens
CREATE TABLE agent_tokens (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    connectors TEXT[] DEFAULT ARRAY['*'],
    methods TEXT[] DEFAULT ARRAY['*'],
    budget_cap_amount_nanos BIGINT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agent_tokens_agent_id ON agent_tokens(agent_id);
CREATE INDEX idx_agent_tokens_expires_at ON agent_tokens(expires_at);

-- credentials — encrypted API keys for connector access
CREATE TABLE credentials (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    env_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    connector_type TEXT NOT NULL,
    account_id TEXT NOT NULL DEFAULT '',
    label TEXT NOT NULL DEFAULT 'primary',
    description TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL DEFAULT 'encrypted',
    encrypted_blob BYTEA,
    key_hash TEXT NOT NULL DEFAULT '',
    wrapping_key_id TEXT NOT NULL DEFAULT '',
    secret_ref TEXT NOT NULL DEFAULT '',
    secret_version TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    version INTEGER NOT NULL DEFAULT 1,
    expires_at TIMESTAMPTZ,
    rotated_at TIMESTAMPTZ,
    rotated_by TEXT NOT NULL DEFAULT '',
    last_used_at TIMESTAMPTZ,
    last_validated_at TIMESTAMPTZ,
    created_by TEXT NOT NULL DEFAULT 'system',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    revoked_by TEXT NOT NULL DEFAULT '',
    revoke_reason TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_credentials_env_id ON credentials(env_id);
CREATE INDEX idx_credentials_connector ON credentials(env_id, connector_type);
