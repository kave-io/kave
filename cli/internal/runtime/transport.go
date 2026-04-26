package runtime

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// bearerCreds implements grpc.PerRPCCredentials by reading the session token
// from the keyring and forwarding it as an Authorization header.
type bearerCreds struct {
	tokenFn func() (string, error)
}

func (b bearerCreds) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	tok, err := b.tokenFn()
	if err != nil || tok == "" {
		return nil, nil
	}
	return map[string]string{"authorization": "Bearer " + tok}, nil
}

func (bearerCreds) RequireTransportSecurity() bool { return false }

func grpcDialContext() context.Context { return context.Background() }

func httpClientFromRuntime(rt *Runtime) *http.Client {
	timeout := 30 * time.Second
	if rt != nil && rt.Resolution != nil {
		if raw := strings.TrimSpace(rt.Resolution.Options.Timeout); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil && d > 0 {
				timeout = d
			}
		}
	}
	return &http.Client{Timeout: timeout}
}

// Transport provides access to both the gRPC clients and the HTTP bridge.
// Command handlers depend on Transport rather than on net/http or grpc directly.
type Transport struct {
	http *HTTPClient

	mu      sync.Mutex
	conn    *grpc.ClientConn
	target  string // host:port derived from the server URL
	useTLS  bool
}

// HTTP returns the HTTP bridge client (used by bridge-only commands).
func (t *Transport) HTTP() *HTTPClient {
	return t.http
}

// RuntimeSvc returns the gRPC RuntimeServiceClient, dialing lazily on first call.
func (t *Transport) RuntimeSvc() (runtimev1.RuntimeServiceClient, error) {
	conn, err := t.grpcConn()
	if err != nil {
		return nil, err
	}
	return runtimev1.NewRuntimeServiceClient(conn), nil
}

// ControlSvc returns the gRPC ControlPlaneServiceClient, dialing lazily on first call.
func (t *Transport) ControlSvc() (controlv1.ControlPlaneServiceClient, error) {
	conn, err := t.grpcConn()
	if err != nil {
		return nil, err
	}
	return controlv1.NewControlPlaneServiceClient(conn), nil
}

// Close releases the gRPC connection if one was opened.
func (t *Transport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn != nil {
		err := t.conn.Close()
		t.conn = nil
		return err
	}
	return nil
}

func (t *Transport) grpcConn() (*grpc.ClientConn, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn != nil {
		return t.conn, nil
	}
	var creds credentials.TransportCredentials
	if t.useTLS {
		creds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	} else {
		creds = insecure.NewCredentials()
	}
	ctx := grpcDialContext()
	conn, err := grpc.NewClient(t.target,
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(bearerCreds{tokenFn: t.http.sessionToken}),
		grpc.WithConnectParams(grpc.ConnectParams{MinConnectTimeout: 5 * time.Second}),
	)
	_ = ctx
	if err != nil {
		return nil, fmt.Errorf("grpc dial %s: %w", t.target, err)
	}
	t.conn = conn
	return conn, nil
}

// NewTransportFromConn builds a Transport backed by an existing gRPC connection.
// Intended for tests (e.g. bufconn-based harnesses) where the connection is
// already established externally.
func NewTransportFromConn(conn *grpc.ClientConn) *Transport {
	return &Transport{
		conn: conn,
		http: &HTTPClient{BaseURL: "http://127.0.0.1:8080", HTTP: &http.Client{}, SessionKey: ""},
	}
}

// newTransport builds a Transport from a Runtime's resolution.
// The server URL must use grpc:// or grpcs:// scheme; legacy http(s):// is rejected.
func newTransport(rt *Runtime) (*Transport, error) {
	raw := resolveBaseURL(rt) // returns http:// form from existing logic
	grpcTarget, useTLS, err := resolveGRPCTarget(rt)
	if err != nil {
		return nil, err
	}
	_ = raw
	http := &HTTPClient{
		BaseURL:    resolveHTTPBridgeURL(rt),
		HTTP:       httpClientFromRuntime(rt),
		SessionKey: sessionAccount(resolveHTTPBridgeURL(rt)),
	}
	return &Transport{
		http:   http,
		target: grpcTarget,
		useTLS: useTLS,
	}, nil
}

// resolveGRPCTarget derives the gRPC host:port from the active server config.
// Canonical form: grpc://host:port or grpcs://host:port.
// Falls back to localhost:<default gRPC port> when no server is configured.
func resolveGRPCTarget(rt *Runtime) (target string, tls bool, err error) {
	const defaultGRPCAddr = "127.0.0.1:9090"

	if rt == nil || rt.Resolution == nil {
		return defaultGRPCAddr, false, nil
	}
	server := strings.TrimSpace(rt.Resolution.ActiveServer())
	if server == "" {
		return defaultGRPCAddr, false, nil
	}

	// Reject legacy http(s):// forms with a clear migration hint.
	if strings.HasPrefix(server, "http://") || strings.HasPrefix(server, "https://") {
		return "", false, fmt.Errorf(
			"server %q uses an http(s):// scheme; migrate to grpc[s]://host:port via `kave ctx`",
			server,
		)
	}

	switch {
	case strings.HasPrefix(server, "grpcs://"):
		return strings.TrimPrefix(server, "grpcs://"), true, nil
	case strings.HasPrefix(server, "grpc://"):
		return strings.TrimPrefix(server, "grpc://"), false, nil
	default:
		// Bare host:port — treat as plain gRPC.
		return server, false, nil
	}
}

// resolveHTTPBridgeURL returns the HTTP bridge base URL derived from the same
// server config. When the server is grpc://host:grpcPort we use the daemon's
// default HTTP bridge port (8080). For legacy http(s):// configs we pass
// through unchanged so bridge-only commands keep working during migration.
func resolveHTTPBridgeURL(rt *Runtime) string {
	if rt == nil || rt.Resolution == nil {
		return "http://127.0.0.1:8080"
	}
	server := strings.TrimSpace(rt.Resolution.ActiveServer())
	if server == "" {
		return "http://127.0.0.1:8080"
	}
	if strings.HasPrefix(server, "http://") || strings.HasPrefix(server, "https://") {
		return strings.TrimRight(server, "/")
	}
	// grpc(s)://host:port — extract host and use default HTTP bridge port.
	host := server
	host = strings.TrimPrefix(host, "grpcs://")
	host = strings.TrimPrefix(host, "grpc://")
	// strip port, use default bridge port
	if idx := strings.LastIndex(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	return "http://" + host + ":8080"
}
