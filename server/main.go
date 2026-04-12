package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	controlmodel "github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/config"
	"github.com/kave-io/kave/server/internal/gateway"
	storeimpl "github.com/kave-io/kave/server/internal/store"
	"github.com/kave-io/kave/server/ops/cost"
	"github.com/kave-io/kave/server/ops/trace"
	portgrpc "github.com/kave-io/kave/server/port/grpc"
	"github.com/kave-io/kave/server/ui"
)

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
	costService, err := cost.NewService(ctx, appStore)
	if err != nil {
		log.Fatalf("create cost service: %v", err)
	}
	grpcServer := portgrpc.New(appStore)

	if err := appStore.Migrate(ctx); err != nil {
		log.Fatalf("app store migrations: %v", err)
	}

	// Create pipeline interceptors in order: cost → trace
	// (auth requires casbin config, skipped for now in local dev)
	costInterceptor := cost.New(appStore, costService)
	traceInterceptor := trace.New(storeManager, costService)

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

	// Register health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	// Serve dashboard SPA at /
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

	if p, _ := app.GetProject(ctx, "default"); p == nil {
		_ = app.CreateProject(ctx, &controlmodel.Project{
			ID: "default", Name: "Default", Slug: "default",
			Description: "Auto-created default project",
			CreatedAt:   now, UpdatedAt: now,
		})
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
			Mode:              controlmodel.PolicyModeEnforce,
			Status:            controlmodel.PolicyStatusActive,
			CreatedAt:         now, UpdatedAt: now,
		})
	}

	if a, _ := app.GetAgentByID(ctx, "default"); a == nil {
		defPolicy := "default"
		budget := money.FromDollars(99999)
		_ = app.CreateAgent(ctx, &controlmodel.Agent{
			ID: "default", ProjectID: "default", EnvID: "default",
			Name:          "default",
			Description:   "Default agent — unauthenticated framework gateway calls are traced here",
			PolicyID:      &defPolicy,
			MonthlyBudget: &budget,
			Status:        controlmodel.AgentStatusActive,
			Metadata:      map[string]any{},
			CreatedAt:     now, UpdatedAt: now,
		})
	}

	// Seed default environment record
	if _, err := app.GetEnvironmentBySlug(ctx, "default", "default"); err != nil {
		_ = app.CreateEnvironment(ctx, &controlmodel.Environment{
			ID:        "default",
			ProjectID: "default",
			Name:      "default",
			Slug:      "default",
			CreatedAt: now,
		})
	}

	// Seed a "default" run for the default agent so we have a seed run_id
	if r, _ := app.GetRunByID(ctx, "default-seed"); r == nil {
		_ = app.CreateRun(ctx, &runtimemodel.RunRecord{
			ID:        "default-seed",
			ProjectID: "default",
			EnvID:     "default",
			AgentID:   "default",
			Name:      "seed",
			Status:    runtimemodel.RunStatusCompleted,
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
