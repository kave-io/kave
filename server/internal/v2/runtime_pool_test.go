package v2

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestValidateTransportSecurity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		mode string
		addr string
		ok   bool
	}{
		{mode: "tls_terminated", addr: ":8080", ok: true},
		{mode: "development", addr: "127.0.0.1:8080", ok: true},
		{mode: "development", addr: "0.0.0.0:8080"},
		{mode: "private_network", addr: "10.1.2.3:8080", ok: true},
		{mode: "private_network", addr: "203.0.113.8:8080"},
		{mode: "private_network", addr: ":8080"},
	}
	for _, test := range tests {
		err := ValidateTransportSecurity(test.mode, test.addr)
		if (err == nil) != test.ok {
			t.Errorf("ValidateTransportSecurity(%q, %q) error = %v, want ok=%v", test.mode, test.addr, err, test.ok)
		}
	}
}

func TestRuntimeDSNRejectsRemotePlaintextFallback(t *testing.T) {
	t.Parallel()
	for _, dsn := range []string{
		"postgres://user:pass@db.example.test/kave?sslmode=disable",
		"postgres://user:pass@db.example.test/kave?sslmode=prefer",
	} {
		config, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateDatabaseTransport(&config.ConnConfig.Config); err == nil {
			t.Fatalf("validateDatabaseTransport(%q) succeeded", dsn)
		}
	}
}
