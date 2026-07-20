package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	v2kernel "github.com/kave-io/kave/server/internal/v2"
	"github.com/kave-io/kave/server/internal/v2/health"
	"github.com/kave-io/kave/server/internal/v2/observability"
	v2postgres "github.com/kave-io/kave/server/internal/v2/postgres"
	"github.com/kave-io/kave/server/ui"
)

var buildVersion = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		slog.Error("kave server stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	command := "serve"
	if len(args) > 0 {
		command = args[0]
	}
	if len(args) > 1 {
		return fmt.Errorf("%s accepts no positional arguments", command)
	}
	switch command {
	case "--version", "version":
		fmt.Fprintf(stdout, "kave-server %s\n", buildVersion)
		return nil
	case "healthz":
		return runHealthProbe(stdout, os.Getenv)
	case "migrate":
		return runV2Migrate()
	case "bootstrap":
		return runV2Bootstrap()
	case "serve":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return runServer(ctx, stdout)
	default:
		return fmt.Errorf("unknown command %q (expected serve, migrate, bootstrap, healthz, or version)", command)
	}
}

func runServer(signalCtx context.Context, stdout io.Writer) error {
	cfg, err := loadRuntimeConfig(os.Getenv)
	if err != nil {
		return fmt.Errorf("load runtime config: %w", err)
	}
	if err := v2kernel.ValidateTransportSecurity(cfg.TransportSecurity, cfg.Address); err != nil {
		return fmt.Errorf("validate HTTP transport boundary: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	startupCtx, startupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer startupCancel()
	pool, err := v2kernel.OpenRuntimePool(startupCtx, cfg.PostgresDSN, cfg.PostgresRole)
	if err != nil {
		return fmt.Errorf("open runtime database: %w", err)
	}
	defer pool.Close()

	metrics := observability.New(func() observability.PoolStats {
		stats := pool.Stat()
		return observability.PoolStats{
			Acquired: stats.AcquiredConns(), Idle: stats.IdleConns(), Total: stats.TotalConns(), Max: stats.MaxConns(),
		}
	})
	lifecycleCtx, stopLifecycle := context.WithCancel(context.Background())
	defer stopLifecycle()
	mux := http.NewServeMux()
	if err := v2kernel.Register(lifecycleCtx, mux, pool, v2kernel.Config{
		MasterKey:                       cfg.MasterKey,
		MasterDecryptionKeys:            cfg.MasterDecryptionKeys,
		SecretIdempotencyKey:            cfg.SecretIdempotencyKey,
		RuntimeRole:                     cfg.PostgresRole,
		ProviderEgressAllowedPrivateIPs: cfg.ProviderEgressAllowedPrivateIPs,
		Metrics:                         metrics,
	}); err != nil {
		return fmt.Errorf("assemble runtime kernel: %w", err)
	}

	health.New(cfg.ReadinessTimeout, []health.Dependency{
		{Name: "postgres_role_migrations", Check: func(ctx context.Context) error {
			return v2postgres.RuntimeReadiness(ctx, pool, cfg.PostgresRole)
		}},
		{Name: "secret_keyring", Check: func(context.Context) error {
			// Register constructed the complete keyring and failed startup if any
			// key was invalid. It is immutable for this process lifetime.
			return nil
		}},
	}, metrics, logger).Register(mux)
	mux.Handle("GET /metrics", metrics.Handler())
	mux.Handle("HEAD /metrics", metrics.Handler())
	mux.Handle("/", ui.Handler())

	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Address, err)
	}
	server := &http.Server{
		Handler:           metrics.HTTPMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// Provider responses can stream for minutes. Shutdown supplies the
		// bounded drain deadline instead of truncating successful responses.
		WriteTimeout:   0,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.Serve(listener)
	}()
	printRuntimeBanner(stdout, listener.Addr().String())
	logger.Info("kave runtime ready", "address", listener.Addr().String(), "version", buildVersion)

	select {
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-signalCtx.Done():
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	shutdownErr := server.Shutdown(shutdownCtx)
	shutdownCancel()
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, server.Close())
	}
	// Active HTTP requests are drained before telemetry workers and Postgres
	// are stopped, so a final settlement is not racing pool closure.
	stopLifecycle()
	serveErr := <-serveErrors
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(shutdownErr, serveErr)
}

const defaultHealthURL = "http://127.0.0.1:8080/readyz"

func runHealthProbe(stdout io.Writer, getenv func(string) string) error {
	endpoint := defaultHealthURL
	if configured := getenv("KAVE_HEALTH_URL"); configured != "" {
		endpoint = configured
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || parsed.Path != "/readyz" || parsed.RawPath != "" {
		return errors.New("KAVE_HEALTH_URL must be an absolute loopback HTTP(S) /readyz URL without credentials, query, or fragment")
	}
	probeIP := net.ParseIP(parsed.Hostname())
	if probeIP == nil || !probeIP.IsLoopback() {
		return errors.New("KAVE_HEALTH_URL must target a loopback IP address")
	}
	client := &http.Client{
		Timeout: 3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create health request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("runtime health request: %w", err)
	}
	_, copyErr := io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	closeErr := response.Body.Close()
	if copyErr != nil || closeErr != nil {
		return errors.Join(copyErr, closeErr)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("runtime is not ready: HTTP %d", response.StatusCode)
	}
	fmt.Fprintln(stdout, "ready")
	return nil
}

func printRuntimeBanner(w io.Writer, address string) {
	host := address
	if strings.HasPrefix(host, "[::]") || strings.HasPrefix(host, "0.0.0.0") {
		_, port, err := net.SplitHostPort(host)
		if err == nil {
			host = net.JoinHostPort("localhost", port)
		}
	}
	fmt.Fprintf(w, "kave %s ready on http://%s\n", buildVersion, host)
}
