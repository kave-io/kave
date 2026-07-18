package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestV2ConfigurationRequiresDedicatedRuntime(t *testing.T) {
	for _, v2 := range []V2Config{
		{Enabled: true, TransportSecurity: "development"},
		{Enabled: true, RuntimeDSN: "postgres://runtime", TransportSecurity: "development"},
		{Enabled: true, RuntimeRole: "kave_runtime", TransportSecurity: "development"},
	} {
		cfg := &Config{V2: v2}
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "runtime_dsn") {
			t.Fatalf("Validate(%+v) error = %v", v2, err)
		}
	}
}

func TestV2ConfigurationRequiresExplicitTransportBoundary(t *testing.T) {
	for _, mode := range []string{"", "insecure", "magic"} {
		cfg := &Config{V2: V2Config{
			Enabled: true, RuntimeDSN: "postgres://runtime", RuntimeRole: "kave_runtime",
			TransportSecurity: mode,
		}}
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "transport_security") {
			t.Fatalf("Validate(mode=%q) error = %v", mode, err)
		}
	}
}

func TestV2ConfigurationAcceptsSeparatedRuntime(t *testing.T) {
	for _, mode := range []string{"tls_terminated", "private_network", "development"} {
		cfg := &Config{V2: V2Config{
			Enabled: true, RuntimeDSN: "postgres://runtime", RuntimeRole: "kave_runtime",
			TransportSecurity: mode,
		}}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate(mode=%q): %v", mode, err)
		}
	}
}

func TestV2ConfigurationAcceptsExactPrivateProviderEgressExceptions(t *testing.T) {
	cfg := &Config{V2: V2Config{
		Enabled: true, RuntimeDSN: "postgres://runtime", RuntimeRole: "kave_runtime",
		TransportSecurity: "development",
		ProviderEgress: V2ProviderEgressConfig{AllowedPrivateIPs: []string{
			"127.0.0.1", "10.20.30.40", "fd00::10",
		}},
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
}

func TestV2ConfigurationRejectsBroadOrUnsafeProviderEgressExceptions(t *testing.T) {
	for _, values := range [][]string{
		{"localhost"},
		{"127.0.0.0/8"},
		{"8.8.8.8"},
		{"169.254.169.254"},
		{"fd00:ec2::254"},
		{"fd20:ce::254"},
		{"::ffff:127.0.0.1"},
		{"127.0.0.1", "127.0.0.1"},
	} {
		cfg := &Config{V2: V2Config{
			Enabled: true, RuntimeDSN: "postgres://runtime", RuntimeRole: "kave_runtime",
			TransportSecurity: "development",
			ProviderEgress:    V2ProviderEgressConfig{AllowedPrivateIPs: values},
		}}
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "provider_egress.allowed_private_ips") {
			t.Fatalf("Validate(%q) error = %v", values, err)
		}
	}
}

func TestLoadV2ProviderEgressExactIPAllowlist(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	configPath := filepath.Join(root, "kave.yaml")
	if err := os.WriteFile(configPath, []byte(`
v2:
  enabled: true
  runtime_dsn: postgres://runtime
  runtime_role: kave_runtime
  transport_security: development
  provider_egress:
    allowed_private_ips:
      - 127.0.0.1
      - fd00::10
`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Load(LoadOpts{StartDir: root})
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	want := []string{"127.0.0.1", "fd00::10"}
	if !slices.Equal(result.Config.V2.ProviderEgress.AllowedPrivateIPs, want) {
		t.Fatalf("allowed private IPs = %q, want %q", result.Config.V2.ProviderEgress.AllowedPrivateIPs, want)
	}
}
