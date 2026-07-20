CREATE SCHEMA IF NOT EXISTS kave_v2;
REVOKE ALL ON SCHEMA kave_v2 FROM PUBLIC;

CREATE FUNCTION kave_v2.set_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = transaction_timestamp();
    RETURN NEW;
END;
$$;

CREATE FUNCTION kave_v2.reject_immutable_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME
        USING ERRCODE = '55000';
END;
$$;

CREATE FUNCTION kave_v2.archive_limit_generation_only()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
	IF OLD.superseded_at IS NOT NULL THEN
		RAISE EXCEPTION 'superseded limit generations are immutable'
			USING ERRCODE = '55000';
	END IF;

	-- Archiving freezes the accounting identity and every policy field. It is
	-- the only mutation that may set superseded_at.
	IF NEW.superseded_at IS NOT NULL THEN
		IF NEW.enabled
			OR (to_jsonb(NEW) - ARRAY['enabled', 'superseded_at', 'updated_at']::TEXT[])
			   <> (to_jsonb(OLD) - ARRAY['enabled', 'superseded_at', 'updated_at']::TEXT[])
		THEN
			RAISE EXCEPTION 'limit generations are immutable; archive and insert a new generation'
				USING ERRCODE = '55000';
		END IF;
		RETURN NEW;
	END IF;

	-- Caps and explicit enablement are mutable policy on a stable accounting
	-- identity. Keeping the ID stable preserves active used/reserved counters
	-- and outstanding provider reservations. Every such update advances the
	-- row revision; selectors, metrics and window definitions remain immutable.
	IF NEW.revision <> OLD.revision + 1
		OR (OLD.source = 'operator' AND NEW.source_version IS DISTINCT FROM OLD.source_version)
		OR (to_jsonb(NEW) - ARRAY['hard_cap', 'soft_cap', 'enabled', 'source_version', 'revision', 'updated_at']::TEXT[])
		   <> (to_jsonb(OLD) - ARRAY['hard_cap', 'soft_cap', 'enabled', 'source_version', 'revision', 'updated_at']::TEXT[])
	THEN
		RAISE EXCEPTION 'limit accounting identity is immutable; update policy or archive and insert a new generation'
			USING ERRCODE = '55000';
	END IF;
	RETURN NEW;
END;
$$;

CREATE FUNCTION kave_v2.archived_agents_are_immutable()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.status = 'archived' THEN
        RAISE EXCEPTION 'archived agent identities are immutable'
            USING ERRCODE = '55000';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

REVOKE ALL ON FUNCTION kave_v2.set_updated_at() FROM PUBLIC;
REVOKE ALL ON FUNCTION kave_v2.reject_immutable_mutation() FROM PUBLIC;
REVOKE ALL ON FUNCTION kave_v2.archive_limit_generation_only() FROM PUBLIC;
REVOKE ALL ON FUNCTION kave_v2.archived_agents_are_immutable() FROM PUBLIC;

-- 1. namespaces: the account + application + environment isolation root.
CREATE TABLE kave_v2.namespaces (
    id              TEXT PRIMARY KEY,
    account_id      TEXT NOT NULL,
    application     TEXT NOT NULL,
    environment     TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'disabled')),
    revision        BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb
                    CHECK (jsonb_typeof(metadata) = 'object'),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    CHECK (id <> '' AND account_id <> '' AND application <> '' AND environment <> ''),
    CHECK (char_length(id) <= 255 AND char_length(account_id) <= 255),
    CHECK (char_length(application) <= 128 AND char_length(environment) <= 128),
    UNIQUE (account_id, id),
    UNIQUE (account_id, application, environment)
);

-- 2. secrets: write-only encrypted material or an external secret URI.
CREATE TABLE kave_v2.secrets (
    id                  TEXT PRIMARY KEY,
    account_id          TEXT NOT NULL,
    namespace_id        TEXT NOT NULL,
    name                TEXT NOT NULL,
    backend             TEXT NOT NULL CHECK (backend IN ('encrypted', 'external')),
    external_ref        TEXT,
    ciphertext          BYTEA,
    wrapping_key_id     TEXT,
    version             BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    status              TEXT NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'revoked', 'invalid')),
    metadata            JSONB NOT NULL DEFAULT '{}'::jsonb
                        CHECK (jsonb_typeof(metadata) = 'object'),
    last_validated_at   TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    revoked_at          TIMESTAMPTZ,
    CHECK (id <> '' AND account_id <> '' AND namespace_id <> '' AND name <> ''),
    CHECK (
        (backend = 'external'
            AND external_ref IS NOT NULL AND external_ref <> ''
            AND ciphertext IS NULL AND wrapping_key_id IS NULL)
        OR
        (backend = 'encrypted'
            AND external_ref IS NULL
            AND ciphertext IS NOT NULL AND octet_length(ciphertext) > 0
            AND wrapping_key_id IS NOT NULL AND wrapping_key_id <> '')
    ),
    CHECK ((status = 'revoked') = (revoked_at IS NOT NULL)),
    UNIQUE (account_id, namespace_id, id),
    UNIQUE (account_id, namespace_id, name),
    FOREIGN KEY (account_id, namespace_id)
        REFERENCES kave_v2.namespaces (account_id, id) ON DELETE CASCADE
);

-- 3. provider_routes: provider routing, model policy, and versioned pricing.
CREATE TABLE kave_v2.provider_routes (
    id                  TEXT PRIMARY KEY,
    account_id          TEXT NOT NULL,
    namespace_id        TEXT NOT NULL,
    name                TEXT NOT NULL,
    provider            TEXT NOT NULL,
    protocol            TEXT NOT NULL DEFAULT 'openai'
                        CHECK (protocol IN ('openai')),
    base_url            TEXT NOT NULL,
    secret_id           TEXT,
    model_policy        JSONB NOT NULL DEFAULT '{}'::jsonb
                        CHECK (jsonb_typeof(model_policy) = 'object'),
    pricing_revision    BIGINT NOT NULL DEFAULT 1 CHECK (pricing_revision > 0),
    pricing             JSONB NOT NULL DEFAULT '{}'::jsonb
                        CHECK (jsonb_typeof(pricing) = 'object'),
    status              TEXT NOT NULL DEFAULT 'invalid'
                        CHECK (status IN ('active', 'disabled', 'invalid', 'archived')),
    revision            BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    last_validated_at   TIMESTAMPTZ,
    validated_secret_version BIGINT CHECK (validated_secret_version > 0),
    validated_model     TEXT,
    validation_evidence JSONB NOT NULL DEFAULT '{}'::jsonb
                        CHECK (jsonb_typeof(validation_evidence) = 'object')
                        CHECK (octet_length(validation_evidence::TEXT) <= 262144),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    CHECK (id <> '' AND account_id <> '' AND namespace_id <> ''),
    CHECK (name <> '' AND provider <> '' AND base_url <> ''),
    CHECK (
        (status = 'active'
            AND last_validated_at IS NOT NULL
            AND validated_secret_version IS NOT NULL
            AND validated_model IS NOT NULL AND validated_model <> '')
        OR status <> 'active'
    ),
    UNIQUE (account_id, namespace_id, id),
    UNIQUE (account_id, namespace_id, name),
    FOREIGN KEY (account_id, namespace_id)
        REFERENCES kave_v2.namespaces (account_id, id) ON DELETE CASCADE,
    FOREIGN KEY (account_id, namespace_id, secret_id)
        REFERENCES kave_v2.secrets (account_id, namespace_id, id) ON DELETE RESTRICT
);

-- 4. agents: static workload definitions, not tenant-specific copies.
CREATE TABLE kave_v2.agents (
    id              TEXT PRIMARY KEY,
    account_id      TEXT NOT NULL,
    namespace_id    TEXT NOT NULL,
    name            TEXT NOT NULL,
    kind            TEXT NOT NULL CHECK (kind IN ('llm', 'embedding')),
    route_id        TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'disabled', 'archived')),
    config          JSONB NOT NULL DEFAULT '{}'::jsonb
                    CHECK (jsonb_typeof(config) = 'object'),
    revision        BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    CHECK (id <> '' AND account_id <> '' AND namespace_id <> '' AND name <> ''),
    UNIQUE (account_id, namespace_id, id),
    FOREIGN KEY (account_id, namespace_id)
        REFERENCES kave_v2.namespaces (account_id, id) ON DELETE CASCADE,
    FOREIGN KEY (account_id, namespace_id, route_id)
        REFERENCES kave_v2.provider_routes (account_id, namespace_id, id) ON DELETE RESTRICT
);

-- Archived agent IDs remain permanent historical workload identities. A name
-- may be reused only by inserting a new current row, so service keys granted
-- to the archived ID never gain authority over the replacement workload.
CREATE UNIQUE INDEX agents_current_name_idx
    ON kave_v2.agents (account_id, namespace_id, name)
    WHERE status <> 'archived';

-- 5. service_keys: namespace-bound machine identities. Raw keys are never stored.
CREATE TABLE kave_v2.service_keys (
    id                  TEXT PRIMARY KEY,
    account_id          TEXT NOT NULL,
    namespace_id        TEXT NOT NULL,
    name                TEXT NOT NULL,
    lookup_prefix       TEXT NOT NULL UNIQUE
                        CHECK (char_length(lookup_prefix) BETWEEN 16 AND 64)
                        CHECK (lookup_prefix ~ '^[A-Za-z0-9_-]+$'),
    secret_hash         BYTEA NOT NULL CHECK (octet_length(secret_hash) = 32),
    capabilities        TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    allowed_agent_ids   TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    can_assert_scope    BOOLEAN NOT NULL DEFAULT FALSE,
    status              TEXT NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'revoked')),
    expires_at          TIMESTAMPTZ,
    last_used_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    revoked_at          TIMESTAMPTZ,
    CHECK (id <> '' AND account_id <> '' AND namespace_id <> '' AND name <> ''),
    CHECK (cardinality(capabilities) BETWEEN 1 AND 8),
    CHECK (array_position(capabilities, NULL) IS NULL),
    CHECK (capabilities <@ ARRAY[
        'consume', 'invoke', 'config.apply', 'secrets.write',
        'keys.manage', 'limits.sync', 'usage.read', 'audit.read'
    ]::TEXT[]),
    CHECK (cardinality(allowed_agent_ids) <= 64),
    CHECK (array_position(allowed_agent_ids, NULL) IS NULL),
    CHECK (array_position(allowed_agent_ids, '') IS NULL),
    CHECK (
        NOT (capabilities && ARRAY['consume', 'invoke']::TEXT[])
        OR cardinality(allowed_agent_ids) > 0
    ),
    CHECK ((status = 'revoked') = (revoked_at IS NOT NULL)),
    UNIQUE (account_id, namespace_id, id),
    UNIQUE (account_id, namespace_id, name),
    FOREIGN KEY (account_id, namespace_id)
        REFERENCES kave_v2.namespaces (account_id, id) ON DELETE CASCADE
);

-- The only pre-RLS identity lookup. Its owner must be the migration owner,
-- distinct from the runtime login role. FORCE RLS still applies; the SELECT
-- policy below opens only the exact random prefix while current_user is the
-- security definer rather than session_user. Runtime roles receive EXECUTE on
-- this function, never unscoped table access.
CREATE FUNCTION kave_v2.lookup_service_key(p_lookup_prefix TEXT)
RETURNS TABLE (
    account_id          TEXT,
    namespace_id        TEXT,
    service_key_id      TEXT,
    secret_hash         BYTEA,
    capabilities        TEXT[],
    allowed_agent_ids   TEXT[],
    can_assert_scope    BOOLEAN,
    status              TEXT,
    expires_at          TIMESTAMPTZ
)
LANGUAGE plpgsql
SECURITY DEFINER
VOLATILE
SET search_path = pg_catalog, pg_temp
SET row_security = on
AS $$
DECLARE
    v_account_id        TEXT;
    v_namespace_id      TEXT;
    v_service_key_id    TEXT;
    v_secret_hash       BYTEA;
    v_capabilities      TEXT[];
    v_allowed_agent_ids TEXT[];
    v_can_assert_scope  BOOLEAN;
    v_status            TEXT;
    v_expires_at        TIMESTAMPTZ;
BEGIN
    IF p_lookup_prefix IS NULL
        OR char_length(p_lookup_prefix) < 16
        OR char_length(p_lookup_prefix) > 64
        OR p_lookup_prefix !~ '^[A-Za-z0-9_-]+$'
    THEN
        RETURN;
    END IF;

    PERFORM pg_catalog.set_config('kave.auth_lookup_prefix', p_lookup_prefix, true);
    SELECT
        service_key.account_id,
        service_key.namespace_id,
        service_key.id,
        service_key.secret_hash,
        service_key.capabilities,
        service_key.allowed_agent_ids,
        service_key.can_assert_scope,
        service_key.status,
        service_key.expires_at
    INTO
        v_account_id, v_namespace_id, v_service_key_id, v_secret_hash,
        v_capabilities, v_allowed_agent_ids, v_can_assert_scope, v_status,
        v_expires_at
    FROM kave_v2.service_keys AS service_key
    WHERE service_key.lookup_prefix = p_lookup_prefix;

    IF NOT FOUND THEN
        RETURN;
    END IF;

    -- Install the resolved scope before checking the namespace kill switch.
    -- FORCE RLS remains active even inside this SECURITY DEFINER function.
    PERFORM pg_catalog.set_config('kave.account_id', v_account_id, true);
    PERFORM pg_catalog.set_config('kave.namespace_id', v_namespace_id, true);

    RETURN QUERY
    SELECT
        v_account_id, v_namespace_id, v_service_key_id, v_secret_hash,
        v_capabilities, v_allowed_agent_ids, v_can_assert_scope, v_status,
        v_expires_at
    WHERE EXISTS (
        SELECT 1
        FROM kave_v2.namespaces AS namespace
        WHERE namespace.account_id = v_account_id
          AND namespace.id = v_namespace_id
          AND namespace.status = 'active'
    );
END;
$$;
REVOKE ALL ON FUNCTION kave_v2.lookup_service_key(TEXT) FROM PUBLIC;

-- 6. limits: hierarchical quotas/budgets with fixed, exact-match selectors.
CREATE TABLE kave_v2.limits (
    id                  TEXT PRIMARY KEY,
    account_id          TEXT NOT NULL,
    namespace_id        TEXT NOT NULL,
    external_key        TEXT NOT NULL,
    generation          BIGINT NOT NULL DEFAULT 1 CHECK (generation > 0),
    source              TEXT NOT NULL DEFAULT 'operator',
    source_version      TEXT NOT NULL DEFAULT '',
    metric              TEXT NOT NULL
                        CHECK (metric ~ '^[a-z][a-z0-9_.-]{0,63}$'),
    currency            TEXT CHECK (currency ~ '^[A-Z]{3}$'),
    tenant_ref          TEXT,
    actor_ref           TEXT,
    billing_ref         TEXT,
    agent_id            TEXT,
    model               TEXT,
    feature             TEXT,
    hard_cap            BIGINT NOT NULL CHECK (hard_cap >= 0),
    soft_cap            BIGINT CHECK (soft_cap >= 0 AND soft_cap <= hard_cap),
    window_kind         TEXT NOT NULL
                        CHECK (window_kind IN ('calendar_day', 'calendar_month', 'fixed', 'explicit', 'lifetime')),
    window_seconds      BIGINT,
    window_anchor       TIMESTAMPTZ,
    effective_from      TIMESTAMPTZ,
    effective_to        TIMESTAMPTZ,
    enabled             BOOLEAN NOT NULL DEFAULT TRUE,
    revision            BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    superseded_at       TIMESTAMPTZ,
    CHECK (id <> '' AND account_id <> '' AND namespace_id <> '' AND external_key <> ''),
    CHECK (char_length(tenant_ref) <= 255 AND char_length(actor_ref) <= 255),
    CHECK (char_length(billing_ref) <= 255 AND char_length(model) <= 255),
    CHECK (char_length(feature) <= 255),
    CHECK (
        (window_kind = 'fixed' AND window_seconds IS NOT NULL AND window_seconds > 0 AND window_anchor IS NOT NULL)
        OR
        (window_kind <> 'fixed' AND window_seconds IS NULL)
    ),
    CHECK (
        window_kind <> 'explicit'
        OR (effective_from IS NOT NULL AND effective_to IS NOT NULL AND effective_to > effective_from)
    ),
    CHECK (effective_to IS NULL OR effective_from IS NULL OR effective_to > effective_from),
    CHECK (superseded_at IS NULL OR NOT enabled),
    UNIQUE (account_id, namespace_id, id),
    UNIQUE (account_id, namespace_id, external_key, generation),
    FOREIGN KEY (account_id, namespace_id)
        REFERENCES kave_v2.namespaces (account_id, id) ON DELETE CASCADE,
    FOREIGN KEY (account_id, namespace_id, agent_id)
        REFERENCES kave_v2.agents (account_id, namespace_id, id) ON DELETE RESTRICT
);

-- One externally named limit is current at a time. Reconciliation keeps the
-- ID stable for cap/soft-cap/enablement policy changes so active counters and
-- reservations survive. A changed selector, metric, or period is a new
-- accounting identity and therefore archives the old generation.
CREATE UNIQUE INDEX limits_current_key_idx
    ON kave_v2.limits (account_id, namespace_id, external_key)
    WHERE superseded_at IS NULL;

CREATE INDEX limits_match_idx
    ON kave_v2.limits (account_id, namespace_id, metric)
    WHERE enabled AND superseded_at IS NULL;
CREATE INDEX limits_tenant_idx
    ON kave_v2.limits (account_id, namespace_id, tenant_ref)
    WHERE enabled AND superseded_at IS NULL AND tenant_ref IS NOT NULL;
CREATE INDEX limits_actor_idx
    ON kave_v2.limits (account_id, namespace_id, actor_ref)
    WHERE enabled AND superseded_at IS NULL AND actor_ref IS NOT NULL;
CREATE INDEX limits_billing_idx
    ON kave_v2.limits (account_id, namespace_id, billing_ref)
    WHERE enabled AND superseded_at IS NULL AND billing_ref IS NOT NULL;

-- 7. limit_windows: the only mutable quota/budget counters.
CREATE TABLE kave_v2.limit_windows (
    account_id      TEXT NOT NULL,
    namespace_id    TEXT NOT NULL,
    limit_id        TEXT NOT NULL,
    window_start    TIMESTAMPTZ NOT NULL,
    window_end      TIMESTAMPTZ NOT NULL,
    used            BIGINT NOT NULL DEFAULT 0 CHECK (used >= 0),
    reserved        BIGINT NOT NULL DEFAULT 0 CHECK (reserved >= 0),
    revision        BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    CHECK (window_end > window_start),
    PRIMARY KEY (account_id, namespace_id, limit_id, window_start),
    UNIQUE (limit_id, window_start),
    FOREIGN KEY (account_id, namespace_id, limit_id)
        REFERENCES kave_v2.limits (account_id, namespace_id, id) ON DELETE CASCADE
);

-- 8. invocations: one logical quota consume or provider call. No payload bodies.
CREATE TABLE kave_v2.invocations (
    id                  TEXT PRIMARY KEY,
    account_id          TEXT NOT NULL,
    namespace_id        TEXT NOT NULL,
    service_key_id      TEXT NOT NULL,
    agent_id            TEXT,
    route_id            TEXT,
    kind                TEXT NOT NULL CHECK (kind IN ('consume', 'provider')),
    operation           TEXT NOT NULL,
    idempotency_key     TEXT NOT NULL,
    request_hash        BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    tenant_ref          TEXT,
    actor_ref           TEXT,
    billing_ref         TEXT,
    session_ref         TEXT,
    feature             TEXT,
    model               TEXT,
    status              TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'admitted', 'rejected', 'settled', 'failed', 'cancelled')),
    rejection_code      TEXT,
    decision            JSONB NOT NULL DEFAULT '{}'::jsonb
                        CHECK (jsonb_typeof(decision) = 'object')
                        CHECK (octet_length(decision::text) <= 16384),
    trace_id            TEXT,
    lease_expires_at    TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    admitted_at         TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    CHECK (id <> '' AND account_id <> '' AND namespace_id <> ''),
    CHECK (service_key_id <> '' AND operation <> '' AND idempotency_key <> ''),
    CHECK (kind <> 'provider' OR (agent_id IS NOT NULL AND route_id IS NOT NULL)),
    CHECK ((status = 'rejected') = (rejection_code IS NOT NULL)),
    CHECK (finished_at IS NULL OR finished_at >= created_at),
    CHECK (char_length(tenant_ref) <= 255 AND char_length(actor_ref) <= 255),
    CHECK (char_length(billing_ref) <= 255 AND char_length(session_ref) <= 255),
    CHECK (char_length(feature) <= 255 AND char_length(model) <= 255),
    UNIQUE (account_id, namespace_id, id),
    UNIQUE (account_id, namespace_id, operation, idempotency_key),
    FOREIGN KEY (account_id, namespace_id)
        REFERENCES kave_v2.namespaces (account_id, id) ON DELETE CASCADE,
    FOREIGN KEY (account_id, namespace_id, service_key_id)
        REFERENCES kave_v2.service_keys (account_id, namespace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (account_id, namespace_id, agent_id)
        REFERENCES kave_v2.agents (account_id, namespace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (account_id, namespace_id, route_id)
        REFERENCES kave_v2.provider_routes (account_id, namespace_id, id) ON DELETE RESTRICT
);

CREATE INDEX invocations_scope_idx
    ON kave_v2.invocations (account_id, namespace_id, created_at DESC);
CREATE INDEX invocations_tenant_idx
    ON kave_v2.invocations (account_id, namespace_id, tenant_ref, created_at DESC)
    WHERE tenant_ref IS NOT NULL;
CREATE INDEX invocations_session_idx
    ON kave_v2.invocations (account_id, namespace_id, session_ref, created_at DESC)
    WHERE session_ref IS NOT NULL;
CREATE INDEX invocations_expired_provider_idx
    ON kave_v2.invocations (account_id, namespace_id, lease_expires_at, id)
    WHERE kind = 'provider'
      AND status IN ('pending', 'admitted')
      AND lease_expires_at IS NOT NULL;

-- 9. usage_entries: immutable reservations, releases, attempts, and usage.
CREATE TABLE kave_v2.usage_entries (
    id                  TEXT PRIMARY KEY,
    account_id          TEXT NOT NULL,
    namespace_id        TEXT NOT NULL,
    invocation_id       TEXT NOT NULL,
    limit_id            TEXT,
    window_start        TIMESTAMPTZ,
    dedupe_key          TEXT NOT NULL,
    event_kind          TEXT NOT NULL
                        CHECK (event_kind IN ('reservation', 'release', 'consume', 'provider_attempt', 'settlement', 'adjustment', 'block')),
    attempt_no          INTEGER NOT NULL DEFAULT 0 CHECK (attempt_no >= 0),
    metric              TEXT,
    quantity            BIGINT NOT NULL DEFAULT 0,
    request_count       BIGINT NOT NULL DEFAULT 0 CHECK (request_count >= 0),
    input_tokens        BIGINT NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens       BIGINT NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    cache_read_tokens   BIGINT NOT NULL DEFAULT 0 CHECK (cache_read_tokens >= 0),
    cache_write_tokens  BIGINT NOT NULL DEFAULT 0 CHECK (cache_write_tokens >= 0),
    reasoning_tokens    BIGINT NOT NULL DEFAULT 0 CHECK (reasoning_tokens >= 0),
    cost_nanos          BIGINT NOT NULL DEFAULT 0 CHECK (cost_nanos >= 0),
    currency            TEXT CHECK (currency ~ '^[A-Z]{3}$'),
    provider            TEXT,
    model               TEXT,
    attempt_status      TEXT,
    http_status         INTEGER CHECK (http_status BETWEEN 100 AND 599),
    latency_ms          BIGINT CHECK (latency_ms >= 0),
    provider_request_id TEXT,
    price_snapshot      JSONB NOT NULL DEFAULT '{}'::jsonb
                        CHECK (jsonb_typeof(price_snapshot) = 'object'),
    usage_detail        JSONB NOT NULL DEFAULT '{}'::jsonb
                        CHECK (jsonb_typeof(usage_detail) = 'object'),
    occurred_at         TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    CHECK (id <> '' AND account_id <> '' AND namespace_id <> ''),
    CHECK (invocation_id <> '' AND dedupe_key <> ''),
    CHECK (event_kind = 'adjustment' OR quantity >= 0),
    CHECK (cost_nanos = 0 OR currency IS NOT NULL),
    CHECK ((limit_id IS NULL) = (window_start IS NULL)),
    UNIQUE (account_id, namespace_id, id),
    UNIQUE (account_id, namespace_id, dedupe_key),
    FOREIGN KEY (account_id, namespace_id, invocation_id)
        REFERENCES kave_v2.invocations (account_id, namespace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (account_id, namespace_id, limit_id)
        REFERENCES kave_v2.limits (account_id, namespace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (limit_id, window_start)
        REFERENCES kave_v2.limit_windows (limit_id, window_start) MATCH FULL ON DELETE RESTRICT
);

CREATE INDEX usage_entries_invocation_idx
    ON kave_v2.usage_entries (account_id, namespace_id, invocation_id, occurred_at);
CREATE INDEX usage_entries_reporting_idx
    ON kave_v2.usage_entries (account_id, namespace_id, occurred_at DESC);

-- 10. audit_events: immutable control and security evidence.
CREATE TABLE kave_v2.audit_events (
    id                  TEXT PRIMARY KEY,
    account_id          TEXT NOT NULL,
    namespace_id        TEXT NOT NULL,
    service_key_id      TEXT,
    event               TEXT NOT NULL,
    resource_type       TEXT NOT NULL,
    resource_id         TEXT NOT NULL,
    outcome             TEXT NOT NULL CHECK (outcome IN ('allowed', 'denied', 'succeeded', 'failed')),
    request_id          TEXT,
    details             JSONB NOT NULL DEFAULT '{}'::jsonb
                        CHECK (jsonb_typeof(details) = 'object')
                        CHECK (octet_length(details::TEXT) <= 65536),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp(),
    CHECK (id <> '' AND account_id <> '' AND namespace_id <> ''),
    CHECK (event <> '' AND resource_type <> '' AND resource_id <> ''),
    UNIQUE (account_id, namespace_id, id),
    FOREIGN KEY (account_id, namespace_id)
        REFERENCES kave_v2.namespaces (account_id, id) ON DELETE CASCADE,
    FOREIGN KEY (account_id, namespace_id, service_key_id)
        REFERENCES kave_v2.service_keys (account_id, namespace_id, id) ON DELETE RESTRICT
);

CREATE INDEX audit_events_query_idx
    ON kave_v2.audit_events (account_id, namespace_id, created_at DESC);

CREATE TRIGGER namespaces_set_updated_at
    BEFORE UPDATE ON kave_v2.namespaces
    FOR EACH ROW EXECUTE FUNCTION kave_v2.set_updated_at();
CREATE TRIGGER secrets_set_updated_at
    BEFORE UPDATE ON kave_v2.secrets
    FOR EACH ROW EXECUTE FUNCTION kave_v2.set_updated_at();
CREATE TRIGGER provider_routes_set_updated_at
    BEFORE UPDATE ON kave_v2.provider_routes
    FOR EACH ROW EXECUTE FUNCTION kave_v2.set_updated_at();
CREATE TRIGGER agents_set_updated_at
    BEFORE UPDATE ON kave_v2.agents
    FOR EACH ROW EXECUTE FUNCTION kave_v2.set_updated_at();
CREATE TRIGGER archived_agents_are_immutable
    BEFORE UPDATE OR DELETE ON kave_v2.agents
    FOR EACH ROW EXECUTE FUNCTION kave_v2.archived_agents_are_immutable();
CREATE TRIGGER service_keys_set_updated_at
    BEFORE UPDATE ON kave_v2.service_keys
    FOR EACH ROW EXECUTE FUNCTION kave_v2.set_updated_at();
CREATE TRIGGER limits_archive_generation_only
    BEFORE UPDATE ON kave_v2.limits
    FOR EACH ROW EXECUTE FUNCTION kave_v2.archive_limit_generation_only();
CREATE TRIGGER limits_set_updated_at
    BEFORE UPDATE ON kave_v2.limits
    FOR EACH ROW EXECUTE FUNCTION kave_v2.set_updated_at();

CREATE TRIGGER usage_entries_are_immutable
    BEFORE UPDATE OR DELETE ON kave_v2.usage_entries
    FOR EACH ROW EXECUTE FUNCTION kave_v2.reject_immutable_mutation();
CREATE TRIGGER audit_events_are_immutable
    BEFORE UPDATE OR DELETE ON kave_v2.audit_events
    FOR EACH ROW EXECUTE FUNCTION kave_v2.reject_immutable_mutation();

ALTER TABLE kave_v2.namespaces ENABLE ROW LEVEL SECURITY;
ALTER TABLE kave_v2.namespaces FORCE ROW LEVEL SECURITY;
CREATE POLICY namespaces_account_isolation ON kave_v2.namespaces
    USING (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND (
            id = NULLIF(current_setting('kave.namespace_id', true), '')
            OR (
                application = NULLIF(current_setting('kave.apply_application', true), '')
                AND environment = NULLIF(current_setting('kave.apply_environment', true), '')
            )
        ))
    WITH CHECK (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND id = NULLIF(current_setting('kave.namespace_id', true), ''));

ALTER TABLE kave_v2.secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE kave_v2.secrets FORCE ROW LEVEL SECURITY;
CREATE POLICY secrets_scope_isolation ON kave_v2.secrets
    USING (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''))
    WITH CHECK (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''));

ALTER TABLE kave_v2.provider_routes ENABLE ROW LEVEL SECURITY;
ALTER TABLE kave_v2.provider_routes FORCE ROW LEVEL SECURITY;
CREATE POLICY provider_routes_scope_isolation ON kave_v2.provider_routes
    USING (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''))
    WITH CHECK (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''));

ALTER TABLE kave_v2.agents ENABLE ROW LEVEL SECURITY;
ALTER TABLE kave_v2.agents FORCE ROW LEVEL SECURITY;
CREATE POLICY agents_scope_isolation ON kave_v2.agents
    USING (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''))
    WITH CHECK (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''));

ALTER TABLE kave_v2.service_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE kave_v2.service_keys FORCE ROW LEVEL SECURITY;
CREATE POLICY service_keys_scope_isolation ON kave_v2.service_keys
    USING (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''))
    WITH CHECK (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''));
CREATE POLICY service_keys_preauth_lookup ON kave_v2.service_keys
    FOR SELECT
    USING (
        current_user <> session_user
        AND lookup_prefix = NULLIF(current_setting('kave.auth_lookup_prefix', true), '')
    );

ALTER TABLE kave_v2.limits ENABLE ROW LEVEL SECURITY;
ALTER TABLE kave_v2.limits FORCE ROW LEVEL SECURITY;
CREATE POLICY limits_scope_isolation ON kave_v2.limits
    USING (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''))
    WITH CHECK (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''));

ALTER TABLE kave_v2.limit_windows ENABLE ROW LEVEL SECURITY;
ALTER TABLE kave_v2.limit_windows FORCE ROW LEVEL SECURITY;
CREATE POLICY limit_windows_scope_isolation ON kave_v2.limit_windows
    USING (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''))
    WITH CHECK (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''));

ALTER TABLE kave_v2.invocations ENABLE ROW LEVEL SECURITY;
ALTER TABLE kave_v2.invocations FORCE ROW LEVEL SECURITY;
CREATE POLICY invocations_scope_isolation ON kave_v2.invocations
    USING (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''))
    WITH CHECK (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''));

ALTER TABLE kave_v2.usage_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE kave_v2.usage_entries FORCE ROW LEVEL SECURITY;
CREATE POLICY usage_entries_scope_isolation ON kave_v2.usage_entries
    USING (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''))
    WITH CHECK (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''));

ALTER TABLE kave_v2.audit_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE kave_v2.audit_events FORCE ROW LEVEL SECURITY;
CREATE POLICY audit_events_scope_isolation ON kave_v2.audit_events
    USING (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''))
    WITH CHECK (account_id = NULLIF(current_setting('kave.account_id', true), '')
        AND namespace_id = NULLIF(current_setting('kave.namespace_id', true), ''));
