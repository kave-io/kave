package main

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	defaultServerAddress    = "127.0.0.1:8080"
	defaultReadinessTimeout = 3 * time.Second
	defaultShutdownTimeout  = 30 * time.Second
)

// runtimeConfig is deliberately environment-only. The serving process has one
// deployment contract instead of implicit config-file discovery, .env files,
// user-home state, or compatibility sections from older products.
type runtimeConfig struct {
	Address                         string
	PostgresDSN                     string
	PostgresRole                    string
	MasterKey                       string
	MasterDecryptionKeys            []string
	SecretIdempotencyKey            string
	TransportSecurity               string
	ProviderEgressAllowedPrivateIPs []string
	ReadinessTimeout                time.Duration
	ShutdownTimeout                 time.Duration
}

func loadRuntimeConfig(getenv func(string) string) (runtimeConfig, error) {
	if getenv == nil {
		return runtimeConfig{}, errors.New("environment reader is required")
	}
	read := func(name string, required bool) (string, error) {
		value := getenv(name)
		if required && value == "" {
			return "", fmt.Errorf("%s is required", name)
		}
		if value != strings.TrimSpace(value) || strings.IndexByte(value, 0) >= 0 || strings.ContainsAny(value, "\r\n") {
			return "", fmt.Errorf("%s must not contain whitespace boundaries, line breaks, or NUL", name)
		}
		return value, nil
	}

	address, err := read("KAVE_SERVER_ADDR", false)
	if err != nil {
		return runtimeConfig{}, err
	}
	if address == "" {
		address = defaultServerAddress
	}
	if err := validateListenAddress(address); err != nil {
		return runtimeConfig{}, err
	}
	postgresDSN, err := read("KAVE_RUNTIME_POSTGRES_DSN", true)
	if err != nil {
		return runtimeConfig{}, err
	}
	postgresRole, err := read("KAVE_RUNTIME_POSTGRES_ROLE", true)
	if err != nil {
		return runtimeConfig{}, err
	}
	masterKey, err := read("KAVE_RUNTIME_MASTER_KEY", true)
	if err != nil {
		return runtimeConfig{}, err
	}
	idempotencyKey, err := read("KAVE_RUNTIME_SECRET_IDEMPOTENCY_KEY", true)
	if err != nil {
		return runtimeConfig{}, err
	}
	transportSecurity, err := read("KAVE_RUNTIME_TRANSPORT_SECURITY", true)
	if err != nil {
		return runtimeConfig{}, err
	}
	decryptionRaw, err := read("KAVE_RUNTIME_MASTER_DECRYPTION_KEYS", false)
	if err != nil {
		return runtimeConfig{}, err
	}
	providerIPsRaw, err := read("KAVE_RUNTIME_PROVIDER_ALLOWED_PRIVATE_IPS", false)
	if err != nil {
		return runtimeConfig{}, err
	}
	readiness, err := runtimeDuration(getenv, "KAVE_RUNTIME_READINESS_TIMEOUT", defaultReadinessTimeout, time.Second, 30*time.Second)
	if err != nil {
		return runtimeConfig{}, err
	}
	shutdown, err := runtimeDuration(getenv, "KAVE_RUNTIME_SHUTDOWN_TIMEOUT", defaultShutdownTimeout, time.Second, 5*time.Minute)
	if err != nil {
		return runtimeConfig{}, err
	}

	return runtimeConfig{
		Address: address, PostgresDSN: postgresDSN, PostgresRole: postgresRole,
		MasterKey: masterKey, MasterDecryptionKeys: exactCSV(decryptionRaw),
		SecretIdempotencyKey: idempotencyKey, TransportSecurity: transportSecurity,
		ProviderEgressAllowedPrivateIPs: exactCSV(providerIPsRaw),
		ReadinessTimeout:                readiness, ShutdownTimeout: shutdown,
	}, nil
}

func validateListenAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		return fmt.Errorf("KAVE_SERVER_ADDR must be a host:port address")
	}
	parsedPort, err := strconv.ParseUint(port, 10, 16)
	if err != nil || parsedPort == 0 {
		return fmt.Errorf("KAVE_SERVER_ADDR port must be between 1 and 65535")
	}
	if strings.ContainsAny(host, "\r\n") {
		return errors.New("KAVE_SERVER_ADDR host is invalid")
	}
	return nil
}

func runtimeDuration(getenv func(string) string, name string, fallback, minimum, maximum time.Duration) (time.Duration, error) {
	raw := getenv(name)
	if raw == "" {
		return fallback, nil
	}
	if raw != strings.TrimSpace(raw) || strings.ContainsAny(raw, "\r\n") {
		return 0, fmt.Errorf("%s is invalid", name)
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be a duration between %s and %s", name, minimum, maximum)
	}
	return value, nil
}

func exactCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	values := strings.Split(raw, ",")
	return slices.Clone(values)
}
