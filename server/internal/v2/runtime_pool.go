package v2

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	v2postgres "github.com/kave-io/kave/server/internal/v2/postgres"
)

// OpenRuntimePool opens V2's dedicated least-privilege pool. A non-loopback
// database must offer encryption on every connection candidate; sslmode=prefer
// is rejected because its plaintext fallback would carry scopes and secrets.
func OpenRuntimePool(ctx context.Context, dsn, expectedRole string) (*pgxpool.Pool, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("v2 kernel: runtime DSN is required")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("v2 kernel: parse runtime DSN: %w", err)
	}
	if err := validateDatabaseTransport(&config.ConnConfig.Config); err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("v2 kernel: open runtime database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("v2 kernel: ping runtime database: %w", err)
	}
	if err := v2postgres.VerifyRuntimeRole(ctx, pool, expectedRole); err != nil {
		pool.Close()
		return nil, fmt.Errorf("v2 kernel: unsafe runtime database role: %w", err)
	}
	return pool, nil
}

// ValidatePostgresDSN applies the same no-plaintext-fallback rule to one-shot
// migration/bootstrap commands before they open a privileged connection.
func ValidatePostgresDSN(dsn string) error {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("v2 kernel: parse Postgres DSN: %w", err)
	}
	return validateDatabaseTransport(&config.ConnConfig.Config)
}

func validateDatabaseTransport(config *pgconn.Config) error {
	if config == nil {
		return errors.New("v2 kernel: runtime database config is unavailable")
	}
	if err := validateDatabaseCandidate(config.Host, config.TLSConfig); err != nil {
		return err
	}
	for _, fallback := range config.Fallbacks {
		if fallback == nil {
			return errors.New("v2 kernel: runtime database contains an invalid fallback")
		}
		if err := validateDatabaseCandidate(fallback.Host, fallback.TLSConfig); err != nil {
			return err
		}
	}
	return nil
}

func validateDatabaseCandidate(host string, tlsConfig *tls.Config) error {
	if isLocalEndpoint(host) {
		return nil
	}
	if tlsConfig == nil {
		return fmt.Errorf("v2 kernel: remote runtime database %q must require TLS without plaintext fallback", host)
	}
	return nil
}

// ValidateTransportSecurity makes the deployment trust boundary explicit.
// tls_terminated is an operator assertion that this listener is reachable only
// through a trusted TLS terminator. Private/development modes also constrain
// the actual bind address so 0.0.0.0 cannot silently expose raw credentials.
func ValidateTransportSecurity(mode, bindAddress string) error {
	if mode == "tls_terminated" {
		return nil
	}
	host, _, err := net.SplitHostPort(bindAddress)
	if err != nil {
		return fmt.Errorf("invalid HTTP bind address %q: %w", bindAddress, err)
	}
	if host == "" {
		return errors.New("unspecified HTTP bind requires KAVE_RUNTIME_TRANSPORT_SECURITY=tls_terminated")
	}
	loopback := isLoopbackHost(host)
	switch mode {
	case "development":
		if !loopback {
			return errors.New("development transport security requires a loopback HTTP bind")
		}
	case "private_network":
		ip := net.ParseIP(strings.Trim(host, "[]"))
		if !loopback && (ip == nil || !ip.IsPrivate()) {
			return errors.New("private_network transport security requires a private or loopback IP bind")
		}
	default:
		return fmt.Errorf("unsupported V2 transport security mode %q", mode)
	}
	return nil
}

func isLocalEndpoint(host string) bool {
	return strings.HasPrefix(host, "/") || isLoopbackHost(host)
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
