package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/intercept"
	"github.com/kave-io/kave/server/config"
	"github.com/kave-io/kave/server/cost"
	postgresinfra "github.com/kave-io/kave/server/infra/postgres"
	"github.com/kave-io/kave/server/proxy"
	storeimpl "github.com/kave-io/kave/server/store"
	"github.com/kave-io/kave/server/trace"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Load config from YAML + environment
	cfg := config.MustReadConfig(".")
	log.Printf("Kave server starting:\n%s", cfg)

	// Setup database (postgres or sqlite, depending on config)
	var pool *pgxpool.Pool
	if cfg.Storage.Backend == "postgres" {
		var err error
		pool, err = postgresinfra.New(ctx, cfg.Postgres)
		if err != nil {
			log.Fatalf("postgres connection: %v", err)
		}
		defer pool.Close()

		// Run migrations
		if err := postgresinfra.Migrate(ctx, pool); err != nil {
			log.Fatalf("postgres migrations: %v", err)
		}
	}

	// Create AppStore and SpanStore (pool may be nil for sqlite/duckdb)
	appStore, err := storeimpl.NewAppStore(cfg.Storage, pool)
	if err != nil {
		log.Fatalf("create app store: %v", err)
	}
	defer appStore.Close()

	spanStore, err := storeimpl.NewSpanStore(cfg.Storage, pool)
	if err != nil {
		log.Fatalf("create span store: %v", err)
	}
	defer spanStore.Close()

	// Run migrations on stores
	if err := appStore.Migrate(ctx); err != nil {
		log.Fatalf("app store migrations: %v", err)
	}
	if err := spanStore.Migrate(ctx); err != nil {
		log.Fatalf("span store migrations: %v", err)
	}

	// Create pipeline interceptors in order: cost → trace
	// (auth requires casbin config, skipped for now in local dev)
	var interceptors []intercept.Interceptor

	if pool != nil {
		costInterceptor := cost.New(pool)
		interceptors = append(interceptors, costInterceptor)
	}

	traceInterceptor := trace.New(spanStore)
	interceptors = append(interceptors, traceInterceptor)

	pipeline := intercept.New(interceptors...)

	// Create and register proxy
	proxyServer := proxy.New(pool, pipeline)
	mux := http.NewServeMux()
	proxyServer.RegisterRoutes(mux)

	// Register health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	// Start HTTP server
	addr := cfg.Server.Addr()
	log.Printf("listening on %s", addr)

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
