// Package testutil provides an in-process integration test harness for the
// Kave server. Call harness.New(t) to boot a full server (SQLite + DuckDB +
// pipeline + gateway) bound to a random port; it cleans up on t.Cleanup.
package testutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/kave-io/kave/core/bus"
	controlmodel "github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/pkg/authhash"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/gateway"
	sqliteStore "github.com/kave-io/kave/server/internal/store/duckdb"
	appStore "github.com/kave-io/kave/server/internal/store/sqlite"
	serverauth "github.com/kave-io/kave/server/ops/auth"
	"github.com/kave-io/kave/server/ops/budget"
	"github.com/kave-io/kave/server/ops/cost"
	"github.com/kave-io/kave/server/ops/policy"
	"github.com/kave-io/kave/server/ops/trace"
)

// IDs used for seeded workspace entities.
const (
	OrgID     = "h_org"
	ProjectID = "h_prj"
	EnvID     = "h_env"
	PolicyID  = "h_pol"
	AgentID   = "h_agt"
	TokenID   = "h_tok"
)

// fixedSpanResolver routes all agents to the same SpanStore.
type fixedSpanResolver struct{ s store.SpanStore }

func (r *fixedSpanResolver) SpanStore(_ context.Context, _ string) (store.SpanStore, error) {
	return r.s, nil
}

// Harness holds all components of an in-process Kave server.
type Harness struct {
	AppStore  store.AppStore
	SpanStore store.SpanStore
	Gateway   *gateway.FrameworkGateway
	Server    *httptest.Server
	Client    *http.Client

	// RawToken is the bearer token for the seeded agent.
	// Use: Authorization: Bearer <RawToken>
	RawToken string
}

// New creates a Harness with a permissive seeded workspace and registers
// cleanup via t.Cleanup. The harness mux is empty of UI/health routes —
// only the gateway routes are registered.
func New(t *testing.T) *Harness {
	t.Helper()

	dir := t.TempDir()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	// App store (SQLite, temp file)
	app, err := appStore.New(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatalf("testutil: sqlite: %v", err)
	}

	// Span store (DuckDB, temp file)
	spans, err := sqliteStore.New(filepath.Join(dir, "spans.db"))
	if err != nil {
		t.Fatalf("testutil: duckdb: %v", err)
	}
	if err := spans.Migrate(ctx); err != nil {
		t.Fatalf("testutil: duckdb migrate: %v", err)
	}

	// Seed workspace
	seed(t, app, ctx, now)

	// Token for seeded agent
	rawToken, tokenHash, err := authhash.GenerateToken("kv_")
	if err != nil {
		t.Fatalf("testutil: generate token: %v", err)
	}
	if err := app.InsertAgentToken(ctx, &controlmodel.AgentToken{
		ID:        TokenID,
		OrgID:     OrgID,
		AgentID:   AgentID,
		ProjectID: ProjectID,
		Name:      "harness-token",
		TokenHash: tokenHash,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("testutil: insert token: %v", err)
	}

	// Pipeline
	eventBus := bus.New()
	costSvc, err := cost.NewService(ctx, app)
	if err != nil {
		t.Fatalf("testutil: cost service: %v", err)
	}
	authIC := serverauth.NewInterceptor(nil, true, true) // anonAllowed=true, legacyAllowed=true
	policyIC := policy.New(app, nil)
	budgetIC := budget.New(app, costSvc)
	traceIC := trace.New(&fixedSpanResolver{spans}, costSvc, eventBus)
	p := pipeline.New(authIC, policyIC, budgetIC, traceIC)

	// Gateway
	gw := gateway.New(app, nil, p, gateway.NewRegistry(), true, nil)

	// HTTP server
	mux := http.NewServeMux()
	gw.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)

	t.Cleanup(func() {
		srv.Close()
		spans.Close()
		app.Close()
	})

	return &Harness{
		AppStore:  app,
		SpanStore: spans,
		Gateway:   gw,
		Server:    srv,
		Client:    srv.Client(),
		RawToken:  rawToken,
	}
}

// seed creates the minimum workspace for integration tests.
func seed(t *testing.T, app store.AppStore, ctx context.Context, now int64) {
	t.Helper()

	must := func(err error, msg string) {
		t.Helper()
		if err != nil {
			t.Fatalf("testutil: seed %s: %v", msg, err)
		}
	}

	must(app.CreateOrg(ctx, &controlmodel.Organization{
		ID: OrgID, Name: "Harness Org", Slug: "harness", Plan: "free",
		CreatedAt: now, UpdatedAt: now,
	}), "org")

	must(app.CreateProject(ctx, &controlmodel.Project{
		ID: ProjectID, OrgID: OrgID, Name: "Harness Project", Slug: "harness",
		CreatedAt: now, UpdatedAt: now,
	}), "project")

	must(app.CreateEnvironment(ctx, &controlmodel.Environment{
		ID: EnvID, ProjectID: ProjectID, Name: "harness-env", Slug: "harness",
		Type: "dev", CreatedAt: now, UpdatedAt: now,
	}), "env")

	polID := PolicyID
	must(app.CreatePolicy(ctx, &controlmodel.PolicyRecord{
		ID:                polID,
		ProjectID:         ProjectID,
		EnvID:             EnvID,
		Name:              "harness-permissive",
		AllowedTypes:      []string{"*"},
		AllowedConnectors: []string{"*"},
		AllowedMethods:    []string{"*"},
		TraceInput:        true,
		TraceOutput:       true,
		RetentionDays:     7,
		Mode:              "enforce",
		Status:            string(controlmodel.PolicyStatusActive),
		CreatedAt:         now, UpdatedAt: now,
	}), "policy")

	must(app.CreateAgent(ctx, &controlmodel.Agent{
		ID:        AgentID,
		ProjectID: ProjectID,
		EnvID:     EnvID,
		Name:      "harness-agent",
		PolicyID:  &polID,
		Status:    controlmodel.AgentStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}), "agent")

	// Credential for openai connector (reads from env var)
	must(app.StoreCredential(ctx, &controlmodel.ConnectorCredential{
		ID:            "h_cred",
		ProjectID:     ProjectID,
		EnvID:         EnvID,
		ConnectorType: "openai",
		Label:         "primary",
		SourceType:    string(controlmodel.CredentialSourceEnv),
		Source:        controlmodel.CredentialSourceEnv,
		EnvVar:        "KAVE_TEST_OPENAI_KEY",
		Status:        controlmodel.CredStatusActive,
		Version:       1,
		CreatedBy:     "system",
		CreatedAt:     now,
		UpdatedAt:     now,
	}), "credential")
}
