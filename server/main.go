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
	corefx "github.com/kave-io/kave/core/fx"
	controlmodel "github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/store"
	appaudit "github.com/kave-io/kave/server/app/audit"
	appcontrol "github.com/kave-io/kave/server/app/control"
	appfx "github.com/kave-io/kave/server/app/fx"
	appruntime "github.com/kave-io/kave/server/app/runtime"
	"github.com/kave-io/kave/server/internal/config"
	"github.com/kave-io/kave/server/internal/daemon"
	"github.com/kave-io/kave/server/internal/gateway"
	appcasbin "github.com/kave-io/kave/server/internal/infra/casbin"
	"github.com/kave-io/kave/server/internal/logsink"
	storeimpl "github.com/kave-io/kave/server/internal/store"
	serverauth "github.com/kave-io/kave/server/ops/auth"
	"github.com/kave-io/kave/server/ops/auth/credresolve"
	"github.com/kave-io/kave/server/ops/budget"
	"github.com/kave-io/kave/server/ops/cost"
	"github.com/kave-io/kave/server/ops/fx"
	"github.com/kave-io/kave/server/ops/policy"
	"github.com/kave-io/kave/server/ops/trace"
	connectport "github.com/kave-io/kave/server/port/connect"
	portgrpc "github.com/kave-io/kave/server/port/grpc"
	"github.com/kave-io/kave/server/ui"
	"net"
)

var buildVersion = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "version":
			fmt.Printf("kave-server %s\n", buildVersion)
			return
		case "healthz":
			fmt.Println("ok")
			return
		}
	}

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
	// core/fx.Service is used by the FX gRPC server (core/fx uses Frankfurter directly).
	corefxService := corefx.NewService(appStore, 0) // 0 → default IRT/USD rate
	if err := corefxService.Load(ctx); err != nil {
		log.Printf("warn: core fx load failed (rates may be stale): %v", err)
	}
	corefxService.StartRefresh(context.Background())
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

	daemonState := daemon.New(config.LoadOpts{StartDir: "."}, loadRes, appStore, storeManager, fxService, costService, eventBus, buildVersion)

	grpcServer := portgrpc.New(
		controlServer,
		runtimeServer,
		auditServer,
		daemonState,
		authTokens,
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

	// Check for permissive environments on public bind
	bindAddr := cfg.Server.Addr()
	if err := checkPermissivePublicBind(context.Background(), appStore, bindAddr); err != nil {
		log.Fatalf("startup check failed: %v", err)
	}
	if isPermissivePublicBind(bindAddr) && os.Getenv("KAVE_ALLOW_PERMISSIVE_PUBLIC") == "1" {
		log.Printf("warn: KAVE_ALLOW_PERMISSIVE_PUBLIC=1 is set — permissive environments will be accessible from the public network")
	}

	// Register FX gRPC service (wraps core/fx.Service).
	appfx.New(corefxService).Register(grpcServer.GRPC())

	go func() {
		if err := grpcServer.ListenAndServe(cfg.GRPC.Addr()); err != nil {
			log.Fatalf("grpc server: %v", err)
		}
	}()

	// Create and register framework gateway
	gatewayServer := gateway.New(appStore, encKey, p, gateway.NewRegistry(), cfg.Security.AllowAnonymous, vaultResolver)
	mux := http.NewServeMux()
	gatewayServer.RegisterRoutes(mux)
	connectport.Register(mux, controlServer, runtimeServer, auditServer, appcontrol.NewDaemonService(daemonState))

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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		now := time.Now().UnixMilli()
		fmt.Fprintf(w, `{"status":"ok","checked_at_ms":%d}`, now)
	})

	mux.Handle("/", ui.Handler())

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

	// Seed default environment record BEFORE policy
	env, _ := app.GetEnvironmentBySlug(ctx, "default", "default")
	if env == nil {
		if err := app.CreateEnvironment(ctx, &controlmodel.Environment{
			ID:        "default",
			ProjectID: "default",
			Name:      "default",
			Slug:      "default",
			Type:      "dev",
			TrustMode: controlmodel.TrustPermissive, // dev env defaults to permissive for ergonomics
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

	if ag, _ := app.GetAgentByName(ctx, "default", "default"); ag == nil {
		policyID := "default"
		_ = app.CreateAgent(ctx, &controlmodel.Agent{
			ID:          "default",
			ProjectID:   "default",
			EnvID:       "default",
			Name:        "default",
			Description: "Default permissive local agent",
			PolicyID:    &policyID,
			Status:      controlmodel.AgentStatusActive,
			Metadata:    map[string]any{},
			CreatedBy:   "system",
			UpdatedBy:   "system",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
}

// isPermissivePublicBind returns true if the bind address is not loopback.
func isPermissivePublicBind(addr string) bool {
	// Extract host from addr (could be "host:port" or just ":port")
	host := addr
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		host = addr[:idx]
	}
	// Empty host or "0.0.0.0" or "::" means public bind
	if host == "" || host == "0.0.0.0" || host == "::" {
		return true
	}
	// Check if it's a loopback address
	ip := net.ParseIP(host)
	return ip != nil && !ip.IsLoopback()
}

// checkPermissivePublicBind ensures no permissive env is exposed to public bind without override.
func checkPermissivePublicBind(ctx context.Context, app store.AppStore, bindAddr string) error {
	// If bind is loopback, always allowed
	if !isPermissivePublicBind(bindAddr) {
		return nil
	}

	// Public bind: check if any environment is permissive
	result, err := app.ListEnvironments(ctx, "", store.Page{Limit: 500})
	if err != nil {
		return err
	}

	var permissiveEnvs []string
	for _, env := range result.Items {
		if env.TrustMode == controlmodel.TrustPermissive {
			permissiveEnvs = append(permissiveEnvs, env.Name)
		}
	}

	// If there are permissive envs and override is not set, refuse to start
	if len(permissiveEnvs) > 0 && os.Getenv("KAVE_ALLOW_PERMISSIVE_PUBLIC") != "1" {
		return fmt.Errorf(
			"cannot start: permissive environments [%s] are exposed to public bind %q without KAVE_ALLOW_PERMISSIVE_PUBLIC=1 override. Set this env var to allow public access to permissive envs (dev/sandbox only)",
			strings.Join(permissiveEnvs, ", "),
			bindAddr,
		)
	}

	return nil
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
