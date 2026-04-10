package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kave-io/kave/core/intercept"
	corestore "github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/config"
	"github.com/kave-io/kave/server/internal/gateway"
	storeimpl "github.com/kave-io/kave/server/internal/store"
	"github.com/kave-io/kave/server/ops/cost"
	"github.com/kave-io/kave/server/ops/trace"
	porthttp "github.com/kave-io/kave/server/port/http"
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

	if err := appStore.Migrate(ctx); err != nil {
		log.Fatalf("app store migrations: %v", err)
	}

	// Create pipeline interceptors in order: cost → trace
	// (auth requires casbin config, skipped for now in local dev)
	var interceptors []intercept.Interceptor

	costInterceptor := cost.New(appStore, costService)
	interceptors = append(interceptors, costInterceptor)

	traceInterceptor := trace.New(storeManager, costService)
	interceptors = append(interceptors, traceInterceptor)

	pipeline := intercept.New(interceptors...)

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

	// Create and register framework gateway
	gatewayServer := gateway.New(appStore, encKey, pipeline)
	mux := http.NewServeMux()
	gatewayServer.RegisterRoutes(mux)

	// Register REST API
	porthttp.New(appStore, storeManager, costService, encKey).RegisterRoutes(mux)

	// Register health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	// Serve dashboard SPA at /
	mux.Handle("/", ui.Handler())

	// Start HTTP server
	addr := cfg.Server.Addr()
	printBanner(addr)

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

// seedDefaults ensures the default workspace, policy, and agent exist.
// All defaults are permissive — event mode: trace everything, no auth or budget limits.
func seedDefaults(ctx context.Context, app corestore.AppStore) {
	now := time.Now().UnixMilli()

	if ws, _ := app.GetWorkspace(ctx, "default"); ws == nil {
		_ = app.CreateWorkspace(ctx, &corestore.Workspace{
			ID: "default", Name: "Default", Slug: "default",
			Description: "Auto-created default workspace",
			CreatedAt:   now, UpdatedAt: now,
		})
	}

	if p, _ := app.GetPolicy(ctx, "default"); p == nil {
		_ = app.CreatePolicy(ctx, &corestore.Policy{
			ID: "default", WorkspaceID: "default",
			Name:              "Default Policy",
			Description:       "Permissive — traces everything, no auth or budget limits",
			AllowedConnectors: []string{"*"},
			AllowedMethods:    []string{"*"},
			CreatedAt:         now, UpdatedAt: now,
		})
	}

	if a, _ := app.GetAgentByID(ctx, "default"); a == nil {
		defPolicy := "default"
		defBudget := 99999.0
		_ = app.CreateAgent(ctx, &corestore.Agent{
			ID: "default", WorkspaceID: "default",
			Name:          "default",
			Description:   "Default agent — unauthenticated framework gateway calls are traced here",
			PolicyID:      &defPolicy,
			MonthlyBudget: &defBudget,
			Metadata:      map[string]any{},
			CreatedAt:     now, UpdatedAt: now,
		})
	}
}

// printBanner prints the startup banner with connection instructions.
func printBanner(addr string) {
	host := addr
	if strings.HasPrefix(addr, ":") {
		host = "localhost" + addr
	}
	base := "http://" + host
	fmt.Printf(`
  ┌─────────────────────────────────────────────────────┐
  │  kave  ready                                        │
  │                                                     │
  │  dashboard  → %s                 │
  │                                                     │
  │  point your AI at the framework gateway             │
  │                                                     │
  │                                                     │
  │  kave watch   →  tail traces in your terminal       │
  └─────────────────────────────────────────────────────┘

`, base)
}
