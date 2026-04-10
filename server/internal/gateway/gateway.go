package gateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/kave-io/kave/connectors/runtime"
	"github.com/kave-io/kave/core/intercept"
	"github.com/kave-io/kave/core/pkg/timex"
	"github.com/kave-io/kave/core/store"
	serverframework "github.com/kave-io/kave/server/internal/connectors/framework"
	"github.com/kave-io/kave/server/internal/infra/crypto"
)

type FrameworkGateway struct {
	app       store.AppStore
	encKey    []byte
	pipeline  *intercept.Pipeline
	transport *HTTPTransport
	claude    serverframework.LLMFamily
}

func New(app store.AppStore, encKey []byte, pipeline *intercept.Pipeline) *FrameworkGateway {
	return &FrameworkGateway{
		app:       app,
		encKey:    encKey,
		pipeline:  pipeline,
		transport: NewHTTPTransport(),
		claude:    serverframework.NewClaudeCodeLLMFamily(),
	}
}

func (g *FrameworkGateway) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/frameworks/claude-code/", g.handleClaudeCode)
}

func (g *FrameworkGateway) handleClaudeCode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	agentID := "default"
	if auth := r.Header.Get("Authorization"); auth != "" {
		if token := strings.TrimPrefix(auth, "Bearer "); token != auth && isUUID(token) {
			agentID = token
		}
	}

	agent, err := g.app.GetAgentByID(ctx, agentID)
	if err != nil || agent == nil {
		http.Error(w, "agent not found", http.StatusUnauthorized)
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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	now := int64(timex.Now())
	run := &store.Run{
		ID:          uuid.NewString(),
		WorkspaceID: agent.WorkspaceID,
		AgentID:     agent.ID,
		Name:        call.Provider + "." + call.Action.Method,
		Status:      "running",
		Metadata:    map[string]any{},
		StartedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := g.app.CreateRun(ctx, run); err != nil {
		http.Error(w, "create run failed", http.StatusInternalServerError)
		return
	}
	call.Action.RunID = run.ID
	call.Action.AgentID = agent.ID
	call.Action.WorkspaceID = agent.WorkspaceID
	if err := g.app.CreateAction(ctx, &store.ActionRecord{
		ID:         call.Action.ID,
		RunID:      run.ID,
		ActionType: string(call.Action.Type),
		Connector:  call.Action.Connector,
		Method:     call.Action.Method,
		Input:      call.Action.Input,
		Metadata:   map[string]any{},
		CreatedAt:  now,
	}); err != nil {
		http.Error(w, "create action failed", http.StatusInternalServerError)
		return
	}

	provider, err := runtime.RequireProvider(g.claude.Providers, call.Provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	credential, _ := g.resolveCredential(ctx, agent.WorkspaceID, call.Provider)

	handler := func(ctx context.Context, action *intercept.Action) (*intercept.Result, error) {
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
			return g.handleStream(w, resp, provider)
		}
		return g.handleBuffered(w, resp, provider)
	}

	result, err := g.pipeline.Execute(ctx, call.Action, handler)
	g.finishRun(ctx, run.ID, err)
	if err != nil && result == nil {
		http.Error(w, fmt.Sprintf("gateway error: %v", err), http.StatusBadGateway)
	}
}

func (g *FrameworkGateway) handleBuffered(w http.ResponseWriter, resp *http.Response, provider runtime.LLMConnector) (*intercept.Result, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)

	return provider.ParseResponse(body, resp.Header.Get("Content-Type"))
}

func (g *FrameworkGateway) handleStream(w http.ResponseWriter, resp *http.Response, provider runtime.LLMConnector) (*intercept.Result, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported by response writer")
	}

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

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

func (g *FrameworkGateway) resolveCredential(ctx context.Context, workspaceID, connector string) (string, error) {
	cred, err := g.app.GetCredential(ctx, workspaceID, connector)
	if err != nil || cred == nil {
		return "", fmt.Errorf("no credential for %s/%s", workspaceID, connector)
	}
	if len(g.encKey) == 0 {
		return string(cred.Encrypted), nil
	}
	aes, err := crypto.NewAES(g.encKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	decrypted, err := aes.Decrypt(cred.Encrypted, []byte(workspaceID))
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
	status := "completed"
	var message *string
	if runErr != nil {
		status = "failed"
		text := runErr.Error()
		message = &text
	}
	_ = g.app.UpdateRun(ctx, runID, &store.RunUpdate{
		Status:       &status,
		ErrorMessage: message,
		EndedAt:      &endedAt,
	})
}
