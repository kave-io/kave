package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kave-io/kave/core/bus"
	controlmodel "github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
	appaudit "github.com/kave-io/kave/server/app/audit"
	appcontrol "github.com/kave-io/kave/server/app/control"
	appruntime "github.com/kave-io/kave/server/app/runtime"
	"github.com/kave-io/kave/server/internal/config"
	"github.com/kave-io/kave/server/internal/contract"
	"github.com/kave-io/kave/server/internal/daemon"
	"github.com/kave-io/kave/server/internal/gateway"
	"github.com/kave-io/kave/server/internal/httpbridge"
	"github.com/kave-io/kave/server/internal/logsink"
	appcasbin "github.com/kave-io/kave/server/internal/infra/casbin"
	storeimpl "github.com/kave-io/kave/server/internal/store"
	serverauth "github.com/kave-io/kave/server/ops/auth"
	"github.com/kave-io/kave/server/ops/auth/credresolve"
	"github.com/kave-io/kave/server/ops/budget"
	"github.com/kave-io/kave/server/ops/cost"
	"github.com/kave-io/kave/server/ops/fx"
	"github.com/kave-io/kave/server/ops/policy"
	"github.com/kave-io/kave/server/ops/trace"
	portgrpc "github.com/kave-io/kave/server/port/grpc"
	"github.com/kave-io/kave/server/ui"
)

var buildVersion = "dev"

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Load config from YAML + environment
	loadRes, err := config.Load(config.LoadOpts{StartDir: "."})
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	cfg := loadRes.Config

	storeManager, err := storeimpl.NewManager(ctx, cfg.Storage, cfg.Postgres)
	if err != nil {
		log.Fatalf("create stores: %v", err)
	}
	defer storeManager.Close()
	appStore := storeManager.AppStore()
	if err := appStore.Migrate(ctx); err != nil {
		log.Fatalf("app store migrations: %v", err)
	}
	costService, err := cost.NewService(ctx, appStore)
	if err != nil {
		log.Fatalf("create cost service: %v", err)
	}
	fxService := fx.NewService(appStore, time.Duration(cfg.FX.RefreshIntervalSeconds)*time.Second)
	if err := fxService.Load(ctx); err != nil {
		log.Fatalf("load fx rates: %v", err)
	}
	if err := fxService.EnsureFresh(ctx); err != nil {
		log.Fatalf("refresh fx rates: %v", err)
	}
	fxService.Start(context.Background())
	if err := storeManager.Migrate(ctx); err != nil {
		log.Fatalf("span store migrations: %v", err)
	}

	authTokens, err := serverauth.NewTokenManager(cfg.Security.EncryptionKey, cfg.Security.SessionTTL, cfg.Security.TokenTTL)
	if err != nil {
		log.Fatalf("create auth tokens: %v", err)
	}
	var vaultResolver *credresolve.VaultResolver
	if cfg.Security.Vault != nil && cfg.Security.Vault.Addr != "" && cfg.Security.Vault.Mount != "" {
		vaultResolver = &credresolve.VaultResolver{
			Addr:  cfg.Security.Vault.Addr,
			Token: cfg.Security.Vault.Token,
			Mount: cfg.Security.Vault.Mount,
		}
	}

	eventBus := bus.New()
	log.SetFlags(0)
	log.SetOutput(logsink.New(os.Stderr, eventBus))

	controlServer := appcontrol.New(appStore, eventBus)
	runtimeServer := appruntime.New(appStore, storeManager, eventBus)
	auditServer := appaudit.New(storeManager.AuditStore())
	grpcServer := portgrpc.New(
		controlServer,
		runtimeServer,
		auditServer,
		portgrpc.NewAuthUnaryInterceptor(appStore, authTokens, cfg.Security.AllowAnonymous, cfg.Security.AllowLegacyTokens),
		portgrpc.NewAuthStreamInterceptor(appStore, authTokens, cfg.Security.AllowAnonymous, cfg.Security.AllowLegacyTokens),
	)

	// Build the casbin engine when Security.Casbin is configured. Nil disables
	// authorization enforcement (auth/policy interceptors fall through gracefully).
	var casbinEngine appcasbin.Casbin
	if cfg.Security.Casbin != nil {
		casbinEngine, err = appcasbin.NewEnforcer(appcasbin.Config{
			CasbinModelPath:  cfg.Security.Casbin.ModelPath,
			DatabaseDSN:      cfg.Security.Casbin.DatabaseDSN,
			SuperAdminBypass: cfg.Security.Casbin.SuperAdminBypass,
		})
		if err != nil {
			log.Fatalf("casbin enforcer: %v", err)
		}
	}

	// Create pipeline interceptors in order: auth → policy → budget → trace.
	authInterceptor := serverauth.NewInterceptor(casbinEngine, cfg.Security.AllowAnonymous, cfg.Security.AllowLegacyTokens)
	policyInterceptor := policy.New(appStore, casbinEngine)
	budgetInterceptor := budget.New(appStore, costService)
	traceInterceptor := trace.New(storeManager, costService, eventBus)

	p := pipeline.New(authInterceptor, policyInterceptor, budgetInterceptor, traceInterceptor)

	// Resolve optional encryption key for credential storage
	var encKey []byte
	if cfg.Security.EncryptionKey != "" {
		encKey, err = hex.DecodeString(cfg.Security.EncryptionKey)
		if err != nil || len(encKey) != 32 {
			log.Fatalf("security.encryption_key must be a 64-char hex string (32 bytes)")
		}
	}

	// Seed default workspace, policy, and agent
	seedDefaults(context.Background(), appStore)

	go func() {
		if err := grpcServer.ListenAndServe(cfg.GRPC.Addr()); err != nil {
			log.Fatalf("grpc server: %v", err)
		}
	}()

	// Create and register framework gateway
	gatewayServer := gateway.New(appStore, encKey, p, gateway.NewRegistry(), cfg.Security.AllowAnonymous, vaultResolver)
	mux := http.NewServeMux()
	gatewayServer.RegisterRoutes(mux)
	fx.RegisterRoutes(mux, fxService)

	bridgeMux := http.NewServeMux()
	httpbridge.Register(bridgeMux, httpbridge.BuildRoutes(controlServer, runtimeServer, appStore, storeManager, authTokens))
	daemonState := daemon.New(config.LoadOpts{StartDir: "."}, loadRes, appStore, storeManager, fxService, costService, eventBus, buildVersion)
	httpbridge.Register(bridgeMux, httpbridge.BuildDaemonRoutes(daemonState))
	httpbridge.RegisterStreams(bridgeMux, appStore, storeManager, eventBus)
	httpbridge.RegisterTraceRoutes(bridgeMux, storeManager)
	if plan, err := daemonState.BuildPlan(context.Background()); err != nil {
		log.Fatalf("build apply plan: %v", err)
	} else if _, err := daemonState.Apply(context.Background(), plan, false); err != nil {
		log.Fatalf("apply config resources: %v", err)
	}
	if err := daemonState.StartWatch(context.Background()); err != nil {
		log.Printf("warn: config watch disabled: %v", err)
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP)
	go func() {
		for range sigCh {
			if _, err := daemonState.Reload(context.Background()); err != nil {
				log.Printf("warn: daemon reload failed: %v", err)
				continue
			}
			log.Printf("info: daemon config reloaded")
		}
	}()

	// Register health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			contract.WriteError(w, http.StatusMethodNotAllowed, "request.method_not_allowed", "method not allowed", nil)
			return
		}
		now := time.Now().UnixMilli()
		contract.WriteSuccess(w, http.StatusOK, "Health", map[string]any{
			"status":        "ok",
			"checked_at":    time.UnixMilli(now).UTC().Format(time.RFC3339Nano),
			"checked_at_ms": now,
		}, nil, nil)
	})

	// Bridge first, then the dashboard SPA.
	root := httpbridge.NewAuthMiddleware(appStore, authTokens, cfg.Security.AllowAnonymous, cfg.Security.AllowLegacyTokens).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := bridgeMux.Handler(r); pattern != "" {
			bridgeMux.ServeHTTP(w, r)
			return
		}
		ui.Handler().ServeHTTP(w, r)
	}))
	mux.Handle("/", root)

	// Start HTTP server
	addr := cfg.Server.Addr()
	printBanner(addr, cfg.GRPC.Addr())

	server := &http.Server{
		Addr:        addr,
		Handler:     mux,
		ReadTimeout: 30 * time.Second,
		// WriteTimeout is intentionally 0 (no timeout) to support streaming
		// responses from the framework gateway which can run for minutes.
		IdleTimeout: 120 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

// seedDefaults ensures the default project, policy, and agent exist.
// All defaults are permissive — event mode: trace everything, no auth or budget limits.
func seedDefaults(ctx context.Context, app store.AppStore) {
	now := time.Now().UnixMilli()

	if o, _ := app.GetOrg(ctx, "default"); o == nil {
		_ = app.CreateOrg(ctx, &controlmodel.Organization{
			ID: "default", Name: "Default", Slug: "default", Plan: "free",
			CreatedAt: now, UpdatedAt: now,
		})
	}

	// Default local user + membership. On self-host they are invisible — the
	// dashboard hides the Org/Users/Members screens while AllowAnonymous=true.
	// Cloud populates real users and makes these UI surfaces visible.
	if u, _ := app.GetUserByEmail(ctx, "default", "local@kave.local"); u == nil {
		_ = app.CreateUser(ctx, &controlmodel.User{
			ID: "default", OrgID: "default", Email: "local@kave.local",
			Name: "Local", Status: "active",
			CreatedAt: now, UpdatedAt: now,
		})
		_ = app.AddMember(ctx, &controlmodel.Membership{
			ID: "default", OrgID: "default", UserID: "default",
			Role: "admin", CreatedAt: now,
		})
	}

	if p, _ := app.GetProject(ctx, "default"); p == nil {
		_ = app.CreateProject(ctx, &controlmodel.Project{
			ID: "default", OrgID: "default", Name: "Default", Slug: "default",
			Description: "Auto-created default project",
			CreatedAt:   now, UpdatedAt: now,
		})
	}

	// Seed default environment record BEFORE policy and agent
	env, _ := app.GetEnvironmentBySlug(ctx, "default", "default")
	if env == nil {
		if err := app.CreateEnvironment(ctx, &controlmodel.Environment{
			ID:        "default",
			ProjectID: "default",
			Name:      "default",
			Slug:      "default",
			Type:      "dev",
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			fmt.Printf("warn: seed default environment: %v\n", err)
		}
	}

	if pol, _ := app.GetPolicy(ctx, "default"); pol == nil {
		_ = app.CreatePolicy(ctx, &controlmodel.PolicyRecord{
			ID: "default", ProjectID: "default", EnvID: "default",
			Name:              "Default Policy",
			Description:       "Permissive — traces everything, no auth or budget limits",
			AllowedTypes:      []string{"*"},
			AllowedConnectors: []string{"*"},
			AllowedMethods:    []string{"*"},
			TraceInput:        true,
			TraceOutput:       true,
			RetentionDays:     30,
			Mode:              "enforce",
			Status:            "active",
			CreatedAt:         now, UpdatedAt: now,
		})
	}

	if a, _ := app.GetAgentByID(ctx, "default"); a == nil {
		defPolicy := "default"
		budget := money.MustParseDollars("99999")
		if err := app.CreateAgent(ctx, &controlmodel.Agent{
			ID: "default", ProjectID: "default", EnvID: "default",
			Name:          "default",
			Description:   "Default agent — unauthenticated framework gateway calls are traced here",
			PolicyID:      &defPolicy,
			MonthlyBudget: &budget,
			Status:        controlmodel.AgentStatusActive,
			Metadata:      map[string]any{},
			CreatedBy:     "system",
			UpdatedBy:     "system",
			CreatedAt:     now, UpdatedAt: now,
		}); err != nil {
			fmt.Printf("warn: seed default agent: %v\n", err)
		}
	}

	// Seed a "default" run for the default agent so we have a seed run_id
	if r, _ := app.GetRunByID(ctx, "default-seed"); r == nil {
		_ = app.CreateRun(ctx, &runtimemodel.RunRecord{
			ID:        "default-seed",
			ProjectID: "default",
			EnvID:     "default",
			AgentID:   "default",
			Name:      "seed",
			Status:    "completed",
			Metadata:  map[string]any{},
			StartedAt: now,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
}

// printBanner prints the startup banner with connection instructions.
func printBanner(addr, grpcAddr string) {
	host := addr
	if strings.HasPrefix(addr, ":") {
		host = "localhost" + addr
	}
	grpcHost := grpcAddr
	if strings.HasPrefix(grpcAddr, ":") {
		grpcHost = "localhost" + grpcAddr
	}
	base := "http://" + host
	fmt.Printf(`
  ┌─────────────────────────────────────────────────────┐
  │  kave  ready                                        │
  │                                                     │
  │  dashboard  → %s                 │
  │  grpc       → %s                 │
  │                                                     │
  │  point your AI at the framework gateway             │
  │                                                     │
  │                                                     │
  │  kave watch   →  tail traces in your terminal       │
  └─────────────────────────────────────────────────────┘

`, base, grpcHost)
}
