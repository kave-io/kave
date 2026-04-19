package gateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kave-io/kave/connectors/runtime"
	controlmodel "github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/pkg/ids"
	"github.com/kave-io/kave/core/pkg/timex"
	coreruntime "github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/store"
	serverframework "github.com/kave-io/kave/server/internal/connectors/framework"
	"github.com/kave-io/kave/server/internal/contract"
	"github.com/kave-io/kave/server/internal/infra/crypto"
)

type FrameworkGateway struct {
	app       store.AppStore
	encKey    []byte
	pipeline  *pipeline.Pipeline
	transport *HTTPTransport
	claude    serverframework.LLMFamily
}

func New(app store.AppStore, encKey []byte, p *pipeline.Pipeline) *FrameworkGateway {
	return &FrameworkGateway{
		app:       app,
		encKey:    encKey,
		pipeline:  p,
		transport: NewHTTPTransport(),
		claude:    serverframework.NewClaudeCodeLLMFamily(),
	}
}

func (g *FrameworkGateway) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/frameworks/claude-code/", g.handleClaudeCode)
}

func (g *FrameworkGateway) handleClaudeCode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tw := &trackingResponseWriter{ResponseWriter: w}

	agentID := "default"
	if auth := r.Header.Get("Authorization"); auth != "" {
		if token := strings.TrimPrefix(auth, "Bearer "); token != auth && isUUID(token) {
			agentID = token
		}
	}

	agent, err := g.app.GetAgentByID(ctx, agentID)
	if err != nil || agent == nil {
		contract.WriteError(tw, http.StatusUnauthorized, "gateway.agent_not_found", "agent not found", nil)
		return
	}

	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}

	call, err := g.claude.Framework.ParseLLMRequest(&runtime.Request{
		Method:   r.Method,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Header:   runtime.CloneHeader(r.Header),
		Body:     body,
	})
	if err != nil {
		contract.WriteError(tw, http.StatusBadRequest, "gateway.bad_request", err.Error(), nil)
		return
	}
	now := int64(timex.Now())
	run := &runtimemodel.RunRecord{
		ID:        ids.New("run"),
		ProjectID: agent.ProjectID,
		EnvID:     agent.EnvID,
		AgentID:   agent.ID,
		Name:      call.Provider + "." + call.Action.InvocationTarget.Method,
		Status:    string(coreruntime.RunActive),
		Metadata:  map[string]any{},
		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := g.app.CreateRun(ctx, run); err != nil {
		contract.WriteError(tw, http.StatusInternalServerError, "gateway.create_run_failed", "create run failed", nil)
		return
	}

	// Set run/agent/project info on the action
	call.Action.Invocation.InvocationRef.RunID = run.ID
	call.Action.Invocation.InvocationRef.AgentID = agent.ID
	call.Action.Invocation.InvocationRef.ProjectID = agent.ProjectID
	call.Action.Invocation.InvocationRef.EnvID = agent.EnvID

	if err := g.app.CreateAction(ctx, &runtimemodel.ActionRecord{
		ID:         call.Action.InvocationRef.ID,
		RunID:      run.ID,
		AgentID:    agent.ID,
		ProjectID:  agent.ProjectID,
		EnvID:      agent.EnvID,
		ActionType: string(call.Action.InvocationTarget.Type),
		Connector:  call.Action.InvocationTarget.Connector,
		Method:     call.Action.InvocationTarget.Method,
		Input:      call.Action.InvocationData.Input,
		Status:     string(coreruntime.StatusRunning),
		Source:     string(coreruntime.ActionSourceIntercepted),
		Metadata:   map[string]any{},
		CreatedAt:  now,
	}); err != nil {
		contract.WriteError(tw, http.StatusInternalServerError, "gateway.create_action_failed", "create action failed", nil)
		return
	}

	provider, err := runtime.RequireProvider(g.claude.Providers, call.Provider)
	if err != nil {
		contract.WriteError(tw, http.StatusBadRequest, "gateway.provider_not_supported", err.Error(), nil)
		return
	}

	credential, credErr := g.resolveCredential(ctx, agent.EnvID, call.Provider)

	// If no stored credential, fallback to client's Authorization header (passthrough mode)
	if credential == "" && credErr != nil {
		if auth := r.Header.Get("Authorization"); auth != "" {
			// Extract token from Bearer scheme
			if token := strings.TrimPrefix(auth, "Bearer "); token != auth {
				credential = token
			} else {
				credential = auth
			}
		}
	}

	handler := func(ctx context.Context, action *coreruntime.Action) (*pipeline.Result, error) {
		prepared, err := provider.PrepareRequest(call, credential)
		if err != nil {
			return nil, err
		}

		resp, err := g.transport.Do(ctx, prepared)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(contentType, "event-stream") {
			return g.handleStream(tw, resp, provider)
		}
		return g.handleBuffered(tw, resp, provider)
	}

	result, err := g.pipeline.Execute(ctx, call.Action, handler)
	g.finishRun(ctx, run.ID, err)
	if err != nil && result == nil && !tw.WroteHeader() {
		contract.WriteError(tw, http.StatusBadGateway, "gateway.upstream_error", fmt.Sprintf("gateway error: %v", err), nil)
	}
}

func (g *FrameworkGateway) handleBuffered(w http.ResponseWriter, resp *http.Response, provider runtime.LLMConnector) (*pipeline.Result, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
	}
	return provider.ParseResponse(body, resp.Header.Get("Content-Type"))
}

func (g *FrameworkGateway) handleStream(w http.ResponseWriter, resp *http.Response, provider runtime.LLMConnector) (*pipeline.Result, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported by response writer")
	}

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream error: %d", resp.StatusCode)
	}

	var buf bytes.Buffer
	tee := io.TeeReader(resp.Body, &buf)
	tmp := make([]byte, 4096)
	for {
		n, readErr := tee.Read(tmp)
		if n > 0 {
			_, _ = w.Write(tmp[:n])
			flusher.Flush()
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("stream upstream: %w", readErr)
		}
	}

	return provider.ParseResponse(buf.Bytes(), resp.Header.Get("Content-Type"))
}

func (g *FrameworkGateway) resolveCredential(ctx context.Context, envID, connector string) (string, error) {
	cred, err := g.app.ResolveCredential(ctx, &controlmodel.CredentialFilter{
		EnvID:         envID,
		ConnectorType: connector,
	})
	if err != nil || cred == nil {
		return "", fmt.Errorf("no credential for %s/%s", envID, connector)
	}
	if len(g.encKey) == 0 {
		return string(cred.EncryptedBlob), nil
	}
	aes, err := crypto.NewAES(g.encKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	decrypted, err := aes.Decrypt(cred.EncryptedBlob, []byte(cred.ProjectID))
	if err != nil {
		return "", fmt.Errorf("decrypt credential: %w", err)
	}
	return string(decrypted), nil
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
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
