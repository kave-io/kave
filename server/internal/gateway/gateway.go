package gateway

import (
	"context"
	"fmt"
	"net/http"

	controlmodel "github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/pkg/timex"
	coreruntime "github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/ops/auth/credresolve"
)

type FrameworkGateway struct {
	app            store.AppStore
	encKey         []byte
	pipeline       *pipeline.Pipeline
	transport      *HTTPTransport
	registry       *Registry
	allowAnonymous bool
	vault          credresolve.VaultClient
}

func New(app store.AppStore, encKey []byte, p *pipeline.Pipeline, registry *Registry, allowAnonymous bool, vault credresolve.VaultClient) *FrameworkGateway {
	if registry == nil {
		registry = NewRegistry()
	}
	return &FrameworkGateway{
		app:            app,
		encKey:         encKey,
		pipeline:       p,
		transport:      NewHTTPTransport(),
		registry:       registry,
		allowAnonymous: allowAnonymous,
		vault:          vault,
	}
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
