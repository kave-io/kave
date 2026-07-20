package gateway

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestProviderDialerRejectsNonPublicAddressesBeforeDial(t *testing.T) {
	t.Parallel()
	blocked := []string{
		"0.0.0.0",
		"10.1.2.3",
		"100.100.100.200",
		"127.0.0.1",
		"168.63.129.16",
		"169.254.169.254",
		"192.0.2.1",
		"198.18.0.1",
		"224.0.0.1",
		"255.255.255.255",
		"::",
		"::1",
		"64:ff9b::808:808",
		"2001:db8::1",
		"fd00::1",
		"fd00:ec2::254",
		"fd20:ce::254",
		"fe80::1",
		"fec0::1",
		"ff02::1",
	}
	for _, raw := range blocked {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			dialCount := 0
			dialer := &providerDialer{
				lookup: func(context.Context, string, string) ([]netip.Addr, error) {
					t.Fatal("literal address unexpectedly reached DNS")
					return nil, nil
				},
				dial: func(context.Context, string, string) (net.Conn, error) {
					dialCount++
					return nil, errors.New("unexpected dial")
				},
				allowedPrivate: map[netip.Addr]struct{}{},
			}
			_, err := dialer.DialContext(context.Background(), "tcp", net.JoinHostPort(raw, "443"))
			if !errors.Is(err, ErrProviderAddressDenied) {
				t.Fatalf("DialContext(%s) error = %v, want address denied", raw, err)
			}
			if dialCount != 0 {
				t.Fatalf("unsafe address reached network dial %d times", dialCount)
			}
		})
	}
}

func TestProviderDialerPinsValidatedDNSAddressAndRejectsMixedAnswers(t *testing.T) {
	t.Parallel()
	dialErr := errors.New("dial stopped for test")
	var target string
	dialer := &providerDialer{
		lookup: func(_ context.Context, network, host string) ([]netip.Addr, error) {
			if network != "ip" || host != "provider.example" {
				t.Fatalf("lookup = %q %q", network, host)
			}
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		},
		dial: func(_ context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" {
				t.Fatalf("network = %q", network)
			}
			target = address
			return nil, dialErr
		},
		allowedPrivate: map[netip.Addr]struct{}{},
	}
	_, err := dialer.DialContext(context.Background(), "tcp", "provider.example:443")
	if !errors.Is(err, dialErr) {
		t.Fatalf("DialContext() error = %v, want fake dial error", err)
	}
	if target != "8.8.8.8:443" {
		t.Fatalf("network target = %q, want pinned resolved IP", target)
	}

	dialCount := 0
	dialer.lookup = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{
			netip.MustParseAddr("8.8.8.8"),
			netip.MustParseAddr("10.0.0.8"),
		}, nil
	}
	dialer.dial = func(context.Context, string, string) (net.Conn, error) {
		dialCount++
		return nil, dialErr
	}
	_, err = dialer.DialContext(context.Background(), "tcp", "provider.example:443")
	if !errors.Is(err, ErrProviderAddressDenied) {
		t.Fatalf("mixed DNS answer error = %v, want address denied", err)
	}
	if dialCount != 0 {
		t.Fatalf("mixed DNS answer reached network dial %d times", dialCount)
	}
}

func TestProviderDialerPrivateOptInIsExact(t *testing.T) {
	t.Parallel()
	allowed, err := parseAllowedPrivateIPs([]string{"127.0.0.1", "10.20.30.40", "fd00::10"})
	if err != nil {
		t.Fatal(err)
	}
	dialErr := errors.New("dial stopped for test")
	var resolved netip.Addr
	var target string
	dialer := &providerDialer{
		lookup: func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{resolved}, nil
		},
		dial: func(_ context.Context, _ string, address string) (net.Conn, error) {
			target = address
			return nil, dialErr
		},
		allowedPrivate: allowed,
	}

	resolved = netip.MustParseAddr("10.20.30.40")
	_, err = dialer.DialContext(context.Background(), "tcp", "self-hosted.example:8443")
	if !errors.Is(err, dialErr) || target != "10.20.30.40:8443" {
		t.Fatalf("allowlisted private dial target=%q error=%v", target, err)
	}

	target = ""
	resolved = netip.MustParseAddr("10.20.30.41")
	_, err = dialer.DialContext(context.Background(), "tcp", "self-hosted.example:8443")
	if !errors.Is(err, ErrProviderAddressDenied) || target != "" {
		t.Fatalf("neighbor private dial target=%q error=%v, want denied", target, err)
	}
}

func TestProviderEgressPolicyRejectsBroadOrPermanentExceptions(t *testing.T) {
	t.Parallel()
	invalid := [][]string{
		{"localhost"},
		{"127.0.0.0/8"},
		{"8.8.8.8"},
		{"169.254.169.254"},
		{"fd00:ec2::254"},
		{"fd20:ce::254"},
		{"::ffff:127.0.0.1"},
		{" 127.0.0.1"},
		{"127.0.0.1", "127.0.0.1"},
	}
	for _, values := range invalid {
		values := values
		t.Run(strings.Join(values, ","), func(t *testing.T) {
			t.Parallel()
			if _, err := NewProviderTransport(ProviderEgressPolicy{AllowedPrivateIPs: values}); err == nil {
				t.Fatalf("NewProviderTransport(%q) succeeded", values)
			}
		})
	}
}

func TestProviderTransportDisablesEnvironmentProxies(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	transport, err := NewProviderTransport(ProviderEgressPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if transport.Proxy != nil || transport.GetProxyConnectHeader != nil || transport.ProxyConnectHeader != nil {
		t.Fatal("provider transport retained proxy configuration")
	}
}

func TestProviderTransportPreservesOriginalTLSServerName(t *testing.T) {
	var serverName string
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	upstream.TLS = &tls.Config{
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			serverName = hello.ServerName
			return nil, nil
		},
	}
	upstream.StartTLS()
	defer upstream.Close()

	host, port, err := net.SplitHostPort(upstream.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	resolved := netip.MustParseAddr(host).Unmap()
	transport, err := newProviderTransport(
		ProviderEgressPolicy{AllowedPrivateIPs: []string{resolved.String()}},
		func(_ context.Context, network, lookupHost string) ([]netip.Addr, error) {
			if network != "ip" || lookupHost != "provider.test" {
				t.Fatalf("lookup = %q %q", network, lookupHost)
			}
			return []netip.Addr{resolved}, nil
		},
		(&net.Dialer{}).DialContext,
	)
	if err != nil {
		t.Fatal(err)
	}
	// The test server has a generated certificate for its literal listener.
	// Verification is disabled only in this unit test so ClientHello SNI can be
	// inspected independently from certificate issuance.
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	client := &http.Client{Transport: transport}
	defer client.CloseIdleConnections()
	response, err := client.Get("https://provider.test:" + port)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if serverName != "provider.test" {
		t.Fatalf("TLS server name = %q, want provider.test", serverName)
	}
}
