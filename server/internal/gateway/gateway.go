package gateway

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	controlmodel "github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/pkg/timex"
	coreruntime "github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/ops/auth/credresolve"
	"golang.org/x/time/rate"
)

type FrameworkGateway struct {
	app       store.AppStore
	encKey    []byte
	pipeline  *pipeline.Pipeline
	transport *HTTPTransport
	registry  *Registry
	vault     credresolve.VaultClient
	// Per-IP rate limiter for guest traffic: map[connector+remoteAddr] -> limiter
	guestLimiters map[string]*rate.Limiter
	limiterMu     sync.Mutex
}

// SetTransport replaces the HTTP transport used for upstream requests.
// Intended for tests only.
func (g *FrameworkGateway) SetTransport(rt http.RoundTripper) {
	g.transport.client.Transport = rt
}

func New(app store.AppStore, encKey []byte, p *pipeline.Pipeline, registry *Registry, _ bool, vault credresolve.VaultClient) *FrameworkGateway {
	if registry == nil {
		registry = NewRegistry()
	}
	return &FrameworkGateway{
		app:           app,
		encKey:        encKey,
		pipeline:      p,
		transport:     NewHTTPTransport(),
		registry:      registry,
		vault:         vault,
		guestLimiters: make(map[string]*rate.Limiter),
	}
}

// getGuestLimiter returns a rate limiter for guest traffic (10 RPS per connector × remote address).
func (g *FrameworkGateway) getGuestLimiter(connector, remoteAddr string) *rate.Limiter {
	key := connector + ":" + remoteAddr
	g.limiterMu.Lock()
	defer g.limiterMu.Unlock()
	if limiter, ok := g.guestLimiters[key]; ok {
		return limiter
	}
	limiter := rate.NewLimiter(10, 10)
	g.guestLimiters[key] = limiter
	return limiter
}

func (g *FrameworkGateway) resolveCredential(ctx context.Context, envID, connector string) (string, error) {
	cred, err := g.app.ResolveCredential(ctx, &controlmodel.CredentialFilter{
		EnvID:         envID,
		ConnectorType: connector,
	})
	if err != nil || cred == nil {
		return "", fmt.Errorf("no credential for %s/%s", envID, connector)
	}
	return credresolve.Resolve(ctx, cred, g.vault)
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

// parseProviderFromPath extracts the provider name from the request path.
// For raw paths (/v1/openai/, /v1/anthropic/, /v1/google/) it returns the segment.
// For framework paths (/frameworks/<name>/<provider>/...) it returns the provider.
func (g *FrameworkGateway) parseProviderFromPath(path, framework string) string {
	if framework == "raw" {
		// path: /v1/<provider>/...
		parts := strings.SplitN(strings.TrimPrefix(path, "/v1/"), "/", 2)
		if len(parts) > 0 && parts[0] != "" {
			return parts[0]
		}
		return ""
	}
	// path: /frameworks/<framework>/<provider>/...
	rest := strings.TrimPrefix(path, "/frameworks/"+framework+"/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return ""
}

// getRemoteAddr extracts the real remote address from the request, respecting X-Forwarded-For.
func getRemoteAddr(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.SplitN(fwd, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// getBindScope returns "loopback" if the request came from a loopback address, else "public".
func getBindScope(r *http.Request) string {
	addr := getRemoteAddr(r)
	ip := net.ParseIP(addr)
	if ip != nil && ip.IsLoopback() {
		return "loopback"
	}
	return "public"
}

func (g *FrameworkGateway) finishRun(ctx context.Context, runID string, runErr error) {
	endedAt := int64(timex.Now())
	status := string(coreruntime.RunCompleted)
	var message *string
	if runErr != nil {
		status = string(coreruntime.RunFailed)
		text := runErr.Error()
		message = &text
	}
	_ = g.app.UpdateRun(ctx, runID, &runtimemodel.RunUpdate{
		Status:       &status,
		ErrorMessage: message,
		EndedAt:      &endedAt,
	})
}

type trackingResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (t *trackingResponseWriter) WriteHeader(statusCode int) {
	t.wroteHeader = true
	t.ResponseWriter.WriteHeader(statusCode)
}

func (t *trackingResponseWriter) Write(b []byte) (int, error) {
	if !t.wroteHeader {
		t.wroteHeader = true
	}
	return t.ResponseWriter.Write(b)
}

func (t *trackingResponseWriter) WroteHeader() bool {
	return t.wroteHeader
}

func (t *trackingResponseWriter) Flush() {
	if f, ok := t.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
