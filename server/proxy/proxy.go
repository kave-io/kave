// Package proxy implements an HTTP proxy for LLM API calls.
// Requests arrive as /proxy/{connector}/... with a Kave agent token.
// The proxy: looks up the agent, retrieves the real API key from credentials,
// runs the intercept pipeline, and forwards to the upstream provider.
package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/kave-io/kave/core/intercept"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/infra/crypto"
)

// Proxy intercepts LLM API calls and runs them through Kave's pipeline.
type Proxy struct {
	app      store.AppStore
	encKey   []byte // 32-byte AES-256 key for credential decryption
	upstream *UpstreamClient
	pipeline *intercept.Pipeline
}

// New creates a new proxy.
// encKey must be 32 bytes (AES-256). If nil, credentials are assumed plaintext.
func New(app store.AppStore, encKey []byte, pipeline *intercept.Pipeline) *Proxy {
	return &Proxy{
		app:      app,
		encKey:   encKey,
		upstream: NewUpstreamClient(),
		pipeline: pipeline,
	}
}

// RegisterRoutes registers proxy handlers on mux.
func (p *Proxy) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/proxy/", p.handleProxy)
}

func (p *Proxy) handleProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Path: /proxy/{connector}/{upstream-path...}
	trimmed := strings.TrimPrefix(r.URL.Path, "/proxy/")
	slash := strings.Index(trimmed, "/")
	if slash < 0 {
		http.Error(w, "invalid proxy path: missing upstream path", http.StatusBadRequest)
		return
	}
	connector := trimmed[:slash]
	upstreamPath := trimmed[slash:] // includes leading /

	// Resolve agent from Authorization header.
	// If the Bearer token is a valid UUID it's a Kave agent ID.
	// Anything else (real API keys like sk-ant-...) → default agent + passthrough.
	agentID := "default"
	if auth := r.Header.Get("Authorization"); auth != "" {
		if token := strings.TrimPrefix(auth, "Bearer "); token != auth && isUUID(token) {
			agentID = token
		}
	}

	agent, err := p.app.GetAgentByID(ctx, agentID)
	if err != nil || agent == nil {
		http.Error(w, "agent not found", http.StatusUnauthorized)
		return
	}

	// Retrieve API key for this connector.
	// Missing credentials → pass through the client's own auth headers.
	apiKey, _ := p.resolveCredential(ctx, agent.WorkspaceID, connector)

	// Buffer body so pipeline can record it as action input.
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}

	// Method is the last path segment (e.g. "completions", "messages").
	segments := strings.Split(strings.Trim(upstreamPath, "/"), "/")
	method := segments[len(segments)-1]

	action := &intercept.Action{
		Unit: intercept.Unit{
			ID:        uuid.NewString(),
			RunID:     agentID,
			Type:      intercept.TypeLLM,
			Connector: connector,
			Method:    method,
			Input:     body,
		},
		Status: intercept.StatusPending,
	}

	// The pipeline handler does the actual upstream call.
	// For streaming responses, it tees the body to w in real time
	// while buffering for token extraction — After hooks still run normally.
	handler := func(ctx context.Context, action *intercept.Action) (*intercept.Result, error) {
		resp, err := p.upstream.Forward(ctx, r, connector, upstreamPath, body, apiKey)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		isStream := strings.Contains(resp.Header.Get("Content-Type"), "event-stream")

		if isStream {
			return p.handleStream(w, resp, connector)
		}
		return p.handleBuffered(w, resp, connector)
	}

	result, err := p.pipeline.Execute(ctx, action, handler)
	if err != nil {
		// Only write error if we haven't started writing the response yet.
		// For streaming, we've already written headers so we can't send an HTTP error.
		if result == nil {
			http.Error(w, fmt.Sprintf("pipeline error: %v", err), http.StatusBadGateway)
		}
	}
}

// handleBuffered reads the full response body, copies headers and status, writes to w.
func (p *Proxy) handleBuffered(w http.ResponseWriter, resp *http.Response, connector string) (*intercept.Result, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(body)

	result := &intercept.Result{Body: body}
	applyTokenUsage(result, body, connector)
	return result, nil
}

// handleStream tees the SSE response to w in real time while buffering for token extraction.
func (p *Proxy) handleStream(w http.ResponseWriter, resp *http.Response, connector string) (*intercept.Result, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported by response writer")
	}

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	var buf bytes.Buffer
	tee := io.TeeReader(resp.Body, &buf)

	tmp := make([]byte, 4096)
	for {
		n, readErr := tee.Read(tmp)
		if n > 0 {
			w.Write(tmp[:n])
			flusher.Flush()
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("stream upstream: %w", readErr)
		}
	}

	buffered := buf.Bytes()
	result := &intercept.Result{Body: buffered}
	applyTokenUsage(result, buffered, connector)
	return result, nil
}

// applyTokenUsage extracts token usage from a response body (JSON or SSE) and sets it on result.
func applyTokenUsage(result *intercept.Result, body []byte, connector string) {
	usage := ExtractTokenUsage(body, connector)
	if len(usage) == 0 {
		// For SSE streams, scan individual data lines
		usage = ExtractStreamingTokenUsage(body, connector)
	}
	if len(usage) == 0 {
		return
	}
	tu := &intercept.TokenUsage{}
	if v, ok := usage["InputTokens"].(int); ok {
		tu.InputTokens = v
	}
	if v, ok := usage["OutputTokens"].(int); ok {
		tu.OutputTokens = v
	}
	if v, ok := usage["CacheRead"].(int); ok {
		tu.CacheRead = v
	}
	if v, ok := usage["CacheWrite"].(int); ok {
		tu.CacheWrite = v
	}
	if v, ok := usage["Model"].(string); ok {
		tu.Model = v
	}
	result.TokenUsage = tu
}

// resolveCredential retrieves and decrypts the API key for a connector.
func (p *Proxy) resolveCredential(ctx context.Context, workspaceID, connector string) (string, error) {
	cred, err := p.app.GetCredential(ctx, workspaceID, connector)
	if err != nil || cred == nil {
		return "", fmt.Errorf("no credential for %s/%s", workspaceID, connector)
	}
	if len(p.encKey) == 0 {
		return string(cred.Encrypted), nil
	}
	aes, err := crypto.NewAES(p.encKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	decrypted, err := aes.Decrypt(cred.Encrypted, []byte(workspaceID))
	if err != nil {
		return "", fmt.Errorf("decrypt credential: %w", err)
	}
	return string(decrypted), nil
}

// isUUID returns true if s looks like a UUID (8-4-4-4-12 hex).
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
