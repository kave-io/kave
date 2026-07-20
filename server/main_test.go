package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunHasFocusedCommandSurface(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := run([]string{"version"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "kave-server") {
		t.Fatalf("version output = %q", output.String())
	}
	for _, args := range [][]string{{"v2-serve"}, {"v2-migrate"}, {"unknown"}, {"version", "extra"}} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("run(%q) succeeded", args)
		}
	}
}

func TestRunHealthProbeUsesDependencyAwareEndpointAndFailsClosed(t *testing.T) {
	t.Parallel()
	var status atomic.Int32
	status.Store(http.StatusOK)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(int(status.Load()))
	}))
	defer server.Close()
	getenv := func(name string) string {
		if name == "KAVE_HEALTH_URL" {
			return server.URL + "/readyz"
		}
		return ""
	}
	var output bytes.Buffer
	if err := runHealthProbe(&output, getenv); err != nil || output.String() != "ready\n" {
		t.Fatalf("ready probe = %q, %v", output.String(), err)
	}
	status.Store(http.StatusServiceUnavailable)
	if err := runHealthProbe(&bytes.Buffer{}, getenv); err == nil {
		t.Fatal("not-ready dependency returned a successful process probe")
	}
}

func TestRunHealthProbeRejectsCredentialBearingURL(t *testing.T) {
	t.Parallel()
	err := runHealthProbe(&bytes.Buffer{}, func(string) string { return "http://user:secret@localhost/readyz" })
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("probe error = %v", err)
	}
}

func TestRunHealthProbeCannotDelegateReadiness(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{
		"https://example.com/readyz",
		"http://localhost/readyz",
		"http://127.0.0.1/livez",
		"http://127.0.0.1/readyz?target=elsewhere",
		"http://127.0.0.1/%72eadyz",
	} {
		if err := runHealthProbe(&bytes.Buffer{}, func(string) string { return endpoint }); err == nil {
			t.Fatalf("probe accepted %q", endpoint)
		}
	}
}

func TestLoadRuntimeConfigHasOneStrictEnvironmentSurface(t *testing.T) {
	t.Parallel()
	values := map[string]string{
		"KAVE_SERVER_ADDR":                          "127.0.0.1:18080",
		"KAVE_RUNTIME_POSTGRES_DSN":                 "postgres://runtime@localhost/kave?sslmode=disable",
		"KAVE_RUNTIME_POSTGRES_ROLE":                "kave_runtime",
		"KAVE_RUNTIME_MASTER_KEY":                   strings.Repeat("a", 64),
		"KAVE_RUNTIME_MASTER_DECRYPTION_KEYS":       strings.Repeat("b", 64) + "," + strings.Repeat("c", 64),
		"KAVE_RUNTIME_SECRET_IDEMPOTENCY_KEY":       strings.Repeat("d", 64),
		"KAVE_RUNTIME_TRANSPORT_SECURITY":           "development",
		"KAVE_RUNTIME_PROVIDER_ALLOWED_PRIVATE_IPS": "127.0.0.1,10.20.30.40",
		"KAVE_RUNTIME_READINESS_TIMEOUT":            "4s",
		"KAVE_RUNTIME_SHUTDOWN_TIMEOUT":             "45s",
	}
	cfg, err := loadRuntimeConfig(func(name string) string { return values[name] })
	if err != nil {
		t.Fatalf("loadRuntimeConfig() error = %v", err)
	}
	if cfg.Address != values["KAVE_SERVER_ADDR"] || cfg.PostgresRole != "kave_runtime" ||
		cfg.ReadinessTimeout != 4*time.Second || cfg.ShutdownTimeout != 45*time.Second ||
		len(cfg.MasterDecryptionKeys) != 2 || len(cfg.ProviderEgressAllowedPrivateIPs) != 2 {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestLoadRuntimeConfigRejectsMissingAmbiguousOrUnsafeValues(t *testing.T) {
	t.Parallel()
	valid := map[string]string{
		"KAVE_RUNTIME_POSTGRES_DSN":           "postgres://runtime@localhost/kave?sslmode=disable",
		"KAVE_RUNTIME_POSTGRES_ROLE":          "kave_runtime",
		"KAVE_RUNTIME_MASTER_KEY":             strings.Repeat("a", 64),
		"KAVE_RUNTIME_SECRET_IDEMPOTENCY_KEY": strings.Repeat("b", 64),
		"KAVE_RUNTIME_TRANSPORT_SECURITY":     "development",
	}
	for _, test := range []struct{ name, value string }{
		{name: "KAVE_RUNTIME_POSTGRES_DSN", value: ""},
		{name: "KAVE_RUNTIME_MASTER_KEY", value: ""},
		{name: "KAVE_RUNTIME_SECRET_IDEMPOTENCY_KEY", value: " secret"},
		{name: "KAVE_SERVER_ADDR", value: "localhost"},
		{name: "KAVE_RUNTIME_READINESS_TIMEOUT", value: "0s"},
	} {
		copyOfValues := make(map[string]string, len(valid)+1)
		for key, value := range valid {
			copyOfValues[key] = value
		}
		copyOfValues[test.name] = test.value
		if _, err := loadRuntimeConfig(func(name string) string { return copyOfValues[name] }); err == nil {
			t.Fatalf("loadRuntimeConfig() accepted %s=%q", test.name, test.value)
		}
	}
}
