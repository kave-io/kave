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
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
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
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    connector TEXT NOT NULL,
    label TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    encrypted BYTEA NOT NULL,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workspace_id, connector, key_hash)
);

CREATE INDEX idx_credentials_workspace ON credentials(workspace_id);
CREATE INDEX idx_credentials_connector ON credentials(workspace_id, connector);
