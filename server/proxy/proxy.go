// Package proxy implements an HTTP proxy for LLM API calls.
// Requests arrive as /proxy/{connector}/... with a Kave agent token.
// The proxy: looks up the agent, retrieves the real API key from credentials,
// runs the intercept pipeline, and forwards to the upstream provider.
package proxy

import (
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
	// e.g. /proxy/openai/v1/chat/completions
	trimmed := strings.TrimPrefix(r.URL.Path, "/proxy/")
	slash := strings.Index(trimmed, "/")
	if slash < 0 {
		http.Error(w, "invalid proxy path: missing upstream path", http.StatusBadRequest)
		return
	}
	connector := trimmed[:slash]
	upstreamPath := trimmed[slash:] // includes leading /

	// Agent token from Authorization header.
	// No token → fallback to default agent (event mode: trace everything).
	agentID := "default"
	if auth := r.Header.Get("Authorization"); auth != "" {
		if token := strings.TrimPrefix(auth, "Bearer "); token != auth {
			agentID = token
		}
	}

	agent, err := p.app.GetAgentByID(ctx, agentID)
	if err != nil || agent == nil {
		http.Error(w, "agent not found", http.StatusUnauthorized)
		return
	}

	// Retrieve API key for this connector.
	// Missing credentials are allowed for local/keyless connectors (e.g. ollama).
	apiKey, _ := p.resolveCredential(ctx, agent.WorkspaceID, connector)

	// Buffer body so pipeline can record it as action input
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}

	// Method is the last path segment (e.g. "completions", "embeddings")
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

	// The pipeline handler does the actual upstream call
	handler := func(ctx context.Context, action *intercept.Action) (*intercept.Result, error) {
		resp, err := p.upstream.Forward(ctx, r, connector, upstreamPath, body, apiKey)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read upstream response: %w", err)
		}

		result := &intercept.Result{Body: respBody}

		usage := ExtractTokenUsage(respBody, connector)
		if len(usage) > 0 {
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

		return result, nil
	}

	result, err := p.pipeline.Execute(ctx, action, handler)
	if err != nil {
		http.Error(w, fmt.Sprintf("pipeline error: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(result.Body)
}

// resolveCredential retrieves and decrypts the API key for a connector.
func (p *Proxy) resolveCredential(ctx context.Context, workspaceID, connector string) (string, error) {
	cred, err := p.app.GetCredential(ctx, workspaceID, connector)
	if err != nil || cred == nil {
		return "", fmt.Errorf("no credential for %s/%s", workspaceID, connector)
	}

	// If no encryption key configured, treat stored bytes as plaintext
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
