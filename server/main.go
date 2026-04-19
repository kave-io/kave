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

	appcontrol "github.com/kave-io/kave/app/control"
	appruntime "github.com/kave-io/kave/app/runtime"
	"github.com/kave-io/kave/core/bus"
	controlmodel "github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/daemon"
	"github.com/kave-io/kave/server/internal/config"
	"github.com/kave-io/kave/server/internal/contract"
	"github.com/kave-io/kave/server/internal/gateway"
	"github.com/kave-io/kave/server/internal/httpbridge"
	"github.com/kave-io/kave/server/internal/logsink"
	storeimpl "github.com/kave-io/kave/server/internal/store"
	"github.com/kave-io/kave/server/ops/cost"
	"github.com/kave-io/kave/server/ops/fx"
	"github.com/kave-io/kave/server/ops/trace"
	portgrpc "github.com/kave-io/kave/server/port/grpc"
	"github.com/kave-io/kave/server/ui"
)

var buildVersion = "dev"

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Load config from YAML + environment
	cfg := config.MustReadConfig(".")

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
	defaultSpanStore, err := storeManager.DefaultSpanStore(context.Background())
	if err != nil {
		log.Fatalf("create default span store: %v", err)
	}

	eventBus := bus.New()
	log.SetFlags(0)
	log.SetOutput(logsink.New(os.Stderr, eventBus))

	controlServer := appcontrol.New(appStore, eventBus)
	runtimeServer := appruntime.New(appStore, defaultSpanStore, eventBus)
	grpcServer := portgrpc.New(controlServer, runtimeServer)

	// Create pipeline interceptors in order: cost → trace
	// (auth requires casbin config, skipped for now in local dev)
	costInterceptor := cost.New(appStore, costService)
	traceInterceptor := trace.New(storeManager, costService, eventBus)

	p := pipeline.New(costInterceptor, traceInterceptor)

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
	gatewayServer := gateway.New(appStore, encKey, p)
	mux := http.NewServeMux()
	gatewayServer.RegisterRoutes(mux)
	fx.RegisterRoutes(mux, fxService)

	bridgeMux := http.NewServeMux()
	httpbridge.Register(bridgeMux, httpbridge.BuildRoutes(controlServer, runtimeServer, appStore, defaultSpanStore))
	daemonState := daemon.New(".", cfg, appStore, defaultSpanStore, fxService, costService, eventBus, buildVersion)
	httpbridge.Register(bridgeMux, httpbridge.BuildDaemonRoutes(daemonState))
	httpbridge.RegisterStreams(bridgeMux, appStore, defaultSpanStore, eventBus)
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
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := bridgeMux.Handler(r); pattern != "" {
			bridgeMux.ServeHTTP(w, r)
			return
		}
		ui.Handler().ServeHTTP(w, r)
	}))

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
