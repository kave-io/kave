package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/pkg/ids"
	corev2 "github.com/kave-io/kave/core/v2"
)

func TestReadCursorBindsQueryAndRange(t *testing.T) {
	t.Parallel()
	window := corev2.TimeRange{From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)}
	token, err := encodeReadCursor(window.From.Add(time.Hour), "use_1", "query-a")
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := decodeReadCursor(token, "query-a", window)
	if err != nil || !cursor.Valid || cursor.ID != "use_1" {
		t.Fatalf("cursor = %+v, err=%v", cursor, err)
	}
	if _, err := decodeReadCursor(token, "query-b", window); !errors.Is(err, corev2.ErrInvalidArgument) {
		t.Fatalf("cross-query cursor error = %v", err)
	}
	if _, err := decodeReadCursor("not base64!", "query-a", window); !errors.Is(err, corev2.ErrInvalidArgument) {
		t.Fatalf("malformed cursor error = %v", err)
	}
}

func TestTenantCursorIsOpaqueAndBoundToReportingQuery(t *testing.T) {
	t.Parallel()
	req := corev2.ListTenantsRequest{
		Caller: corev2.Caller{AccountID: "account/acme", NamespaceID: "namespace/prod"},
		Range: corev2.TimeRange{
			From: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	query := tenantQueryFingerprint(req)
	token, err := encodeTenantCursor("clinic/one", "billing/one", query)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := decodeTenantCursor(token, query)
	if err != nil || !cursor.Valid || cursor.Tenant != "clinic/one" || cursor.BillTo != "billing/one" {
		t.Fatalf("cursor = %+v, err=%v", cursor, err)
	}

	changedRange := req
	changedRange.Range.To = changedRange.Range.To.Add(-time.Millisecond)
	changedNamespace := req
	changedNamespace.Caller.NamespaceID = "namespace/other"
	for name, fingerprint := range map[string]string{
		"range":     tenantQueryFingerprint(changedRange),
		"namespace": tenantQueryFingerprint(changedNamespace),
	} {
		if _, err := decodeTenantCursor(token, fingerprint); !errors.Is(err, corev2.ErrInvalidArgument) {
			t.Fatalf("%s-bound cursor error = %v", name, err)
		}
	}

	unknownField := base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"t":"clinic/one","b":"billing/one","q":"` + query + `","extra":true}`))
	invalidRef := base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"t":"clinic/one\nforged","b":"billing/one","q":"` + query + `"}`))
	trailingDocument := base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"t":"clinic/one","b":"billing/one","q":"` + query + `"}{}`))
	for name, malformed := range map[string]string{
		"not base64":        "not base64!",
		"unknown field":     unknownField,
		"invalid reference": invalidRef,
		"trailing document": trailingDocument,
	} {
		if _, err := decodeTenantCursor(malformed, query); !errors.Is(err, corev2.ErrInvalidArgument) {
			t.Fatalf("%s cursor error = %v", name, err)
		}
	}
}

func TestSafeAuditMetadataIsScalarAndSecretDenylisted(t *testing.T) {
	t.Parallel()
	metadata := safeAuditMetadata([]byte(`{"count":2,"ok":true,"name":"agent","note":"Bearer leaked-value","provider_note":"sk-abcdefghijklmnop","secret":"no","nested":{"x":1},"token_hash":"no","Authorization":"no","x-api-key":"no","auth.header":"no","cookie":"no","request_body":"no","prompt":"no"}`))
	if metadata["count"] != "2" || metadata["ok"] != "true" || metadata["name"] != "agent" {
		t.Fatalf("metadata = %#v", metadata)
	}
	for _, key := range []string{"secret", "token_hash", "Authorization", "x-api-key", "auth.header", "cookie", "request_body", "prompt"} {
		if _, ok := metadata[key]; ok {
			t.Fatalf("sensitive metadata %q leaked: %#v", key, metadata)
		}
	}
	if _, ok := metadata["nested"]; ok {
		t.Fatalf("nested metadata was flattened: %#v", metadata)
	}
	if metadata["note"] != "[redacted]" || metadata["provider_note"] != "[redacted]" {
		t.Fatalf("credential-shaped metadata value leaked: %#v", metadata)
	}
	if got := safeAuditMetadata([]byte(`{"safe":"value"}{}`)); got != nil {
		t.Fatalf("trailing audit document accepted: %#v", got)
	}
	if got := safeAuditMetadata(make([]byte, (64<<10)+1)); got != nil {
		t.Fatalf("oversized audit document accepted: %#v", got)
	}
}

func TestManifestStatePreservesDesiredResourcesWhileRouteAwaitsActivation(t *testing.T) {
	t.Parallel()
	state := &applyState{
		routes: map[string]routeRow{
			"ready":    {name: "ready", provider: "openai", status: "active"},
			"awaiting": {name: "awaiting", provider: "openai", status: "invalid"},
			"retired":  {name: "retired", provider: "openai", status: "archived"},
		},
		agents: map[string]agentRow{
			"enabled":  {name: "enabled", routeName: "ready", status: "active"},
			"disabled": {name: "disabled", routeName: "awaiting", status: "disabled"},
			"retired":  {name: "retired", routeName: "retired", status: "archived"},
		},
	}
	routes := manifestRoutes(state)
	agents := manifestAgents(state)
	if len(routes) != 2 || routes[0].Name != "awaiting" || routes[1].Name != "ready" {
		t.Fatalf("routes = %+v", routes)
	}
	if len(agents) != 2 || agents[0].Name != "disabled" || agents[0].Enabled ||
		agents[1].Name != "enabled" || !agents[1].Enabled {
		t.Fatalf("agents = %+v", agents)
	}
}

func TestReadStorePostgres_StateStatusAndScopedPagination(t *testing.T) {
	dsn := os.Getenv("KAVE_TEST_V2_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KAVE_TEST_V2_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	migrator, err := NewMigrator(pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	fixture := seedReadFixture(t, ctx, pool)
	store, err := NewReadStore(pool)
	if err != nil {
		t.Fatal(err)
	}

	state, err := store.GetState(ctx, corev2.GetStateRequest{Caller: fixture.admin, NamespaceID: fixture.namespaceID})
	if err != nil {
		t.Fatal(err)
	}
	if state.NamespaceID != fixture.namespaceID || state.Revision != 4 ||
		len(state.Manifest.Routes) != 1 || len(state.Manifest.Routes[0].Pricing) != 1 ||
		state.Manifest.Routes[0].PricingRevision != 7 || len(state.Manifest.Agents) != 1 ||
		len(state.Manifest.Limits) != 2 {
		t.Fatalf("state = %+v", state)
	}

	status, err := store.GetLimitStatus(ctx, corev2.GetLimitStatusRequest{
		Caller: fixture.worker, Scope: fixture.scope, Agent: "assistant", Metric: "ai_actions",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 1 || status[0].Used != 3 || status[0].Reserved != 2 || status[0].HardCap != 10 {
		t.Fatalf("status = %+v", status)
	}

	rangeFilter := corev2.TimeRange{From: fixture.now.Add(-time.Hour), To: fixture.now.Add(time.Hour)}
	usageRequest := corev2.QueryUsageRequest{
		Caller: fixture.admin, Scope: fixture.scope, Range: rangeFilter, Page: corev2.Page{Size: 1},
	}
	first, err := store.QueryUsage(ctx, usageRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != 1 || first.NextPageToken == "" {
		t.Fatalf("first page = %+v", first)
	}
	usageRequest.Page.Token = first.NextPageToken
	second, err := store.QueryUsage(ctx, usageRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Entries) != 1 || second.NextPageToken != "" || second.Entries[0].ID == first.Entries[0].ID {
		t.Fatalf("second page = %+v; first = %+v", second, first)
	}
	var preserved bool
	for _, entry := range append(first.Entries, second.Entries...) {
		if entry.CostNanoUSD == 42 && entry.Estimated && entry.Provider == "openai" && entry.Model == "gpt-safe" &&
			entry.Attempt == 2 && entry.RequestCount == 1 && entry.InputTokens == 12 &&
			entry.OutputTokens == 3 && entry.CacheReadTokens == 4 {
			preserved = true
		}
	}
	if !preserved {
		t.Fatalf("provider dimensions were not preserved: %+v %+v", first.Entries, second.Entries)
	}
	usageRequest.Page = corev2.Page{Size: 10}
	usageRequest.Metric = corev2.MetricInputTokens
	inputUsage, err := store.QueryUsage(ctx, usageRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputUsage.Entries) != 1 || inputUsage.Entries[0].Metric != corev2.MetricInputTokens ||
		inputUsage.Entries[0].Quantity != 12 {
		t.Fatalf("input usage projection = %+v", inputUsage)
	}
	usageRequest.Metric = "ai_actions"
	productUsage, err := store.QueryUsage(ctx, usageRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(productUsage.Entries) != 1 || productUsage.Entries[0].Quantity != 1 ||
		productUsage.Entries[0].Metric != "ai_actions" {
		t.Fatalf("product usage projection = %+v", productUsage)
	}
	usageRequest.Metric = ""

	invocations, err := store.QueryInvocations(ctx, corev2.QueryInvocationsRequest{
		Caller: fixture.admin, Scope: fixture.scope, Range: rangeFilter, Page: corev2.Page{Size: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(invocations.Invocations) != 2 || invocations.Invocations[0].Decision != corev2.DecisionAdmitted ||
		invocations.Invocations[0].Scope.Tenant != fixture.scope.Tenant {
		t.Fatalf("invocations = %+v", invocations)
	}

	tenantRequest := corev2.ListTenantsRequest{
		Caller: fixture.admin, Range: rangeFilter, Page: corev2.Page{Size: 1},
	}
	var tenantSummaries []corev2.TenantSummary
	for pageNumber := 0; pageNumber < 4; pageNumber++ {
		page, err := store.ListTenants(ctx, tenantRequest)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Tenants) != 1 {
			t.Fatalf("tenant page %d = %+v", pageNumber, page)
		}
		tenantSummaries = append(tenantSummaries, page.Tenants...)
		if page.NextPageToken == "" {
			break
		}
		tenantRequest.Page.Token = page.NextPageToken
	}
	if len(tenantSummaries) != 3 || tenantSummaries[0].Tenant != "clinic/limit-only" ||
		tenantSummaries[1].Tenant != "clinic/observed" || tenantSummaries[2].Tenant != "clinic/one" {
		t.Fatalf("tenant summaries = %+v", tenantSummaries)
	}
	limitOnly, observed, active := tenantSummaries[0], tenantSummaries[1], tenantSummaries[2]
	if limitOnly.Status != corev2.TenantStatusActive || limitOnly.ActiveLimits != 1 || limitOnly.LastSeenAt != nil ||
		limitOnly.InvocationCount != 0 || limitOnly.RequestCount != 0 || limitOnly.CostNanoUSD != 0 {
		t.Fatalf("limit-only tenant = %+v", limitOnly)
	}
	if observed.Status != corev2.TenantStatusObserved || observed.ActiveLimits != 0 || observed.LastSeenAt == nil ||
		observed.InvocationCount != 1 || observed.RequestCount != 0 || observed.CostNanoUSD != 0 {
		t.Fatalf("observed tenant = %+v", observed)
	}
	if active.Status != corev2.TenantStatusActive || active.ActiveLimits != 1 || active.LastSeenAt == nil ||
		active.InvocationCount != 2 || active.RequestCount != 1 || active.CostNanoUSD != 42 {
		t.Fatalf("active tenant = %+v", active)
	}

	audit, err := store.QueryAuditEvents(ctx, corev2.QueryAuditEventsRequest{
		Caller: fixture.admin, Range: rangeFilter, Page: corev2.Page{Size: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(audit.Events) != 1 || audit.Events[0].Metadata["safe"] != "visible" {
		t.Fatalf("audit = %+v", audit)
	}
	if _, leaked := audit.Events[0].Metadata["secret"]; leaked {
		t.Fatalf("audit leaked denylisted metadata: %+v", audit.Events[0])
	}

	otherScope := fixture.scope
	otherScope.Tenant = "clinic/other"
	usageRequest.Scope = otherScope
	usageRequest.Page = corev2.Page{Size: 10}
	empty, err := store.QueryUsage(ctx, usageRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Entries) != 0 {
		t.Fatalf("cross-tenant usage leaked: %+v", empty)
	}
}

type readFixture struct {
	admin, worker corev2.Caller
	namespaceID   corev2.Ref
	scope         corev2.Scope
	now           time.Time
}

func seedReadFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) readFixture {
	t.Helper()
	accountID := corev2.Ref(ids.New("acc"))
	namespaceID := corev2.Ref(ids.New("nsp"))
	secretID := ids.New("sec")
	routeID := ids.New("rte")
	archivedRouteID := ids.New("rte")
	agentID := corev2.Ref(ids.New("agt"))
	archivedAgentID := corev2.Ref(ids.New("agt"))
	adminID := corev2.Ref(ids.New("key"))
	workerID := corev2.Ref(ids.New("key"))
	limitID := ids.New("lim")
	invocationID := ids.New("ivk")
	consumeInvocationID := ids.New("ivk")
	observedInvocationID := ids.New("ivk")
	limitOnlyID := ids.New("lim")
	otherNamespaceID := corev2.Ref(ids.New("nsp"))
	now := time.Now().UTC().Truncate(time.Microsecond)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	scope := Scope{AccountID: string(accountID), NamespaceID: string(namespaceID)}
	runner, err := NewScopedRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(t.Name()))
	policy, _ := json.Marshal(routeModelPolicy{AllowedModels: []string{"gpt-safe"}, DefaultModel: "gpt-safe"})
	pricing, _ := json.Marshal(routePricingDocument([]corev2.ModelPrice{{Model: "gpt-safe", InputNanosPerMillionTokens: 2, OutputNanosPerMillionTokens: 7}}))
	details := []byte(`{"safe":"visible","secret":"hidden","nested":{"omitted":true}}`)
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.namespaces (id, account_id, application, environment, revision)
VALUES ($1, $2, 'simorq', 'test', 4)
`, namespaceID, accountID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.secrets (id, account_id, namespace_id, name, backend, external_ref)
VALUES ($1, $2, $3, 'provider-key', 'external', 'vault://test/provider')
`, secretID, accountID, namespaceID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.provider_routes
  (id, account_id, namespace_id, name, provider, base_url, secret_id, model_policy, pricing_revision, pricing,
   status, last_validated_at, validated_secret_version, validated_model)
VALUES ($1, $2, $3, 'openai', 'openai', 'https://api.openai.com/v1', $4, $5, 7, $6,
        'active', transaction_timestamp(), 1, 'gpt-safe')
`, routeID, accountID, namespaceID, secretID, policy, pricing); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.provider_routes
  (id, account_id, namespace_id, name, provider, base_url, secret_id, model_policy, pricing_revision, pricing, status)
VALUES ($1, $2, $3, 'retired-route', 'openai', 'https://api.openai.com/v1', $4, $5, 7, $6, 'archived')
`, archivedRouteID, accountID, namespaceID, secretID, policy, pricing); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.agents (id, account_id, namespace_id, name, kind, route_id)
VALUES ($1, $2, $3, 'assistant', 'llm', $4)
`, agentID, accountID, namespaceID, routeID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.agents (id, account_id, namespace_id, name, kind, route_id, status)
VALUES ($1, $2, $3, 'retired-agent', 'llm', $4, 'archived')
`, archivedAgentID, accountID, namespaceID, routeID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.service_keys
  (id, account_id, namespace_id, name, lookup_prefix, secret_hash, capabilities)
VALUES ($1, $2, $3, 'admin', $4, $5, ARRAY['config.apply', 'usage.read', 'audit.read'])
`, adminID, accountID, namespaceID, "lookup_"+ids.New("pfx"), hash[:]); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.service_keys
  (id, account_id, namespace_id, name, lookup_prefix, secret_hash, capabilities, allowed_agent_ids, can_assert_scope)
VALUES ($1, $2, $3, 'worker', $4, $5, ARRAY['consume'], ARRAY[$6], TRUE)
`, workerID, accountID, namespaceID, "lookup_"+ids.New("pfx"), hash[:], agentID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.limits
  (id, account_id, namespace_id, external_key, source, metric, tenant_ref, billing_ref, agent_id, hard_cap, soft_cap, window_kind)
VALUES ($1, $2, $3, 'clinic-actions', 'operator', 'ai_actions', 'clinic/one', 'clinic/one', $4, 10, 8, 'calendar_month')
`, limitID, accountID, namespaceID, agentID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.limits
  (id, account_id, namespace_id, external_key, source, metric, tenant_ref, billing_ref, hard_cap, window_kind)
VALUES ($1, $2, $3, 'limit-only-actions', 'operator', 'ai_actions',
        'clinic/limit-only', 'billing/limit-only', 5, 'lifetime')
`, limitOnlyID, accountID, namespaceID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.limit_windows
  (account_id, namespace_id, limit_id, window_start, window_end, used, reserved)
VALUES ($1, $2, $3, $4, $5, 3, 2)
`, accountID, namespaceID, limitID, monthStart, monthStart.AddDate(0, 1, 0)); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.invocations
  (id, account_id, namespace_id, service_key_id, agent_id, route_id, kind, operation,
   idempotency_key, request_hash, tenant_ref, billing_ref, model, status, admitted_at, finished_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, 'provider', 'invoke', $7, $8,
        'clinic/one', 'clinic/one', 'gpt-safe', 'settled', $9, $10, $9)
`, invocationID, accountID, namespaceID, workerID, agentID, routeID, ids.New("once"), hash[:], now.Add(-time.Minute), now); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.invocations
  (id, account_id, namespace_id, service_key_id, agent_id, kind, operation,
   idempotency_key, request_hash, tenant_ref, billing_ref, model, status,
   admitted_at, finished_at, created_at)
VALUES ($1, $2, $3, $4, $5, 'consume', 'consume', $6, $7,
        'clinic/one', 'clinic/one', 'gpt-safe', 'settled', $8, $8, $8)
`, consumeInvocationID, accountID, namespaceID, workerID, agentID,
			ids.New("once"), hash[:], now.Add(-45*time.Second)); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.invocations
  (id, account_id, namespace_id, service_key_id, agent_id, kind, operation,
   idempotency_key, request_hash, tenant_ref, billing_ref, status,
   admitted_at, finished_at, created_at)
VALUES ($1, $2, $3, $4, $5, 'consume', 'consume', $6, $7,
        'clinic/observed', 'billing/observed', 'settled', $8, $8, $8)
`, observedInvocationID, accountID, namespaceID, workerID, agentID,
			ids.New("once"), hash[:], now.Add(-15*time.Second)); err != nil {
			return err
		}
		// Internal per-limit evidence deliberately duplicates the logical unit.
		// QueryUsage must exclude it from billing totals.
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.usage_entries
  (id, account_id, namespace_id, invocation_id, limit_id, window_start,
   dedupe_key, event_kind, metric, quantity, usage_detail, occurred_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'consume', 'ai_actions', 1,
        '{"entry_role":"limit_window"}'::jsonb, $8)
`, ids.New("use"), accountID, namespaceID, consumeInvocationID, limitID,
			monthStart, ids.New("dedupe"), now.Add(-40*time.Second)); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.usage_entries
	  (id, account_id, namespace_id, invocation_id, dedupe_key, event_kind,
	   metric, quantity, usage_detail, occurred_at)
	VALUES ($1, $2, $3, $4, $5, 'consume', 'ai_actions', 1,
	        '{"entry_role":"logical"}'::jsonb, $6)
`, ids.New("use"), accountID, namespaceID, consumeInvocationID,
			"consume:"+consumeInvocationID, now.Add(-30*time.Second)); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.usage_entries
	  (id, account_id, namespace_id, invocation_id, dedupe_key, event_kind, attempt_no,
	   request_count, input_tokens, output_tokens, cache_read_tokens,
	   cost_nanos, currency, provider, model, usage_detail, occurred_at)
	VALUES ($1, $2, $3, $4, $5, 'settlement', 2,
	        1, 12, 3, 4, 42, 'USD', 'openai', 'gpt-safe',
	        '{"accounting_estimate":true}'::jsonb, $6)
`, ids.New("use"), accountID, namespaceID, invocationID,
			"usage:"+invocationID+":2", now); err != nil {
			return err
		}
		_, err := db.Exec(ctx, `
INSERT INTO kave_v2.audit_events
  (id, account_id, namespace_id, service_key_id, event, resource_type, resource_id, outcome, details, created_at)
VALUES ($1, $2, $3, $4, 'gateway.settle', 'invocation', $5, 'succeeded', $6, $7)
`, ids.New("aud"), accountID, namespaceID, workerID, invocationID, details, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	otherScope := Scope{AccountID: string(accountID), NamespaceID: string(otherNamespaceID)}
	if err := runner.WithScope(ctx, otherScope, func(ctx context.Context, db DBTX) error {
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.namespaces (id, account_id, application, environment, revision)
VALUES ($1, $2, 'simorq', 'other-test-namespace', 1)
`, otherNamespaceID, accountID); err != nil {
			return err
		}
		_, err := db.Exec(ctx, `
INSERT INTO kave_v2.limits
  (id, account_id, namespace_id, external_key, source, metric, tenant_ref, billing_ref, hard_cap, window_kind)
VALUES ($1, $2, $3, 'cross-namespace-actions', 'operator', 'ai_actions',
        'clinic/must-not-leak', 'billing/must-not-leak', 5, 'lifetime')
`, ids.New("lim"), accountID, otherNamespaceID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return readFixture{
		admin: corev2.Caller{
			AccountID: accountID, NamespaceID: namespaceID, ServiceKeyID: adminID,
			Operations: []corev2.Operation{corev2.OperationConfigApply, corev2.OperationUsageRead, corev2.OperationAuditRead}, CanAssertScope: true,
		},
		worker:      corev2.Caller{AccountID: accountID, NamespaceID: namespaceID, ServiceKeyID: workerID, Operations: []corev2.Operation{corev2.OperationConsume}, AllowedAgentIDs: []corev2.Ref{agentID}, CanAssertScope: true},
		namespaceID: namespaceID, scope: corev2.Scope{Tenant: "clinic/one", BillTo: "clinic/one"}, now: now,
	}
}
