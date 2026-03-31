package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/intercept"
	"github.com/kave-io/kave/server/infra/crypto"
)

// Proxy intercepts LLM API calls and runs them through Kave's pipeline.
type Proxy struct {
	pool           *pgxpool.Pool
	upstream       *UpstreamClient
	pipeline       *intercept.Pipeline
	credentialMgr  *CredentialManager
}

// New creates a new proxy.
func New(pool *pgxpool.Pool, pipeline *intercept.Pipeline) *Proxy {
	return &Proxy{
		pool:          pool,
		upstream:      NewUpstreamClient(),
		pipeline:      pipeline,
		credentialMgr: NewCredentialManager(pool),
	}
}

// RegisterRoutes registers proxy handlers with a standard HTTP mux.
// Expects routes like /proxy/openai/v1/chat/completions
func (p *Proxy) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/proxy/", p.handleProxy)
}

// handleProxy is the main proxy handler.
func (p *Proxy) handleProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse path: /proxy/{connector}/{path...}
	path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(path, "/proxy/"), "/")
	if len(parts) < 2 {
		http.Error(w, "invalid proxy path", http.StatusBadRequest)
		return
	}

	connector := parts[0]
	method := parts[len(parts)-1] // Last segment is usually the method (e.g., "completions")

	// Extract agent from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "missing authorization", http.StatusUnauthorized)
		return
	}

	agentID := strings.TrimPrefix(authHeader, "Bearer ")
	if agentID == authHeader {
		http.Error(w, "invalid authorization format", http.StatusUnauthorized)
		return
	}

	// Get workspace_id from agent (for credential lookup)
	var workspaceID string
	err := p.pool.QueryRow(ctx, `SELECT workspace_id FROM agents WHERE id = $1`, agentID).Scan(&workspaceID)
	if err != nil {
		http.Error(w, "agent not found", http.StatusUnauthorized)
		return
	}

	// Get credentials for this connector
	cred, err := p.credentialMgr.GetCredential(ctx, workspaceID, connector)
	if err != nil {
		http.Error(w, "credential not found", http.StatusForbidden)
		return
	}

	// Parse request body to extract input
	var input map[string]interface{}
	if r.Body != nil {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &input)
		r.Body = io.NopCloser(strings.NewReader(string(body)))
	}

	// Create Action for the pipeline
	action := &intercept.Action{
		ID:        uuid.New().String(),
		RunID:     agentID, // Use agent ID as run ID for now; proper runs created elsewhere
		Type:      intercept.ActionTypeLLMCall,
		Connector: connector,
		Method:    method,
		Input:     input,
		Metadata: map[string]interface{}{
			"http_method": r.Method,
			"path":        path,
		},
	}

	// Execute pipeline with upstream as the handler
	handler := func(ctx context.Context, action *intercept.Action) (*intercept.Result, error) {
		// Forward to upstream
		resp, err := p.upstream.Forward(r, connector, cred)
		if err != nil {
			return &intercept.Result{
				ActionID: action.ID,
				Error:    err,
			}, nil
		}
		defer resp.Body.Close()

		// Read response body
		respBody, _ := io.ReadAll(resp.Body)

		// Extract token usage
		tokenData := ExtractTokenUsage(respBody, connector)

		var tokens *intercept.TokenUsage
		if tokenData != nil && len(tokenData) > 0 {
			tokens = &intercept.TokenUsage{}
			if v, ok := tokenData["InputTokens"]; ok {
				tokens.InputTokens = v.(int)
			}
			if v, ok := tokenData["OutputTokens"]; ok {
				tokens.OutputTokens = v.(int)
			}
			if v, ok := tokenData["CacheRead"]; ok {
				tokens.CacheRead = v.(int)
			}
			if v, ok := tokenData["CacheWrite"]; ok {
				tokens.CacheWrite = v.(int)
			}
			if v, ok := tokenData["Model"]; ok {
				tokens.Model = v.(string)
			}
		}

		output := map[string]interface{}{
			"status_code": resp.StatusCode,
		}
		json.Unmarshal(respBody, &output)

		return &intercept.Result{
			ActionID: action.ID,
			Output:   output,
			Tokens:   tokens,
		}, nil
	}

	result, err := p.pipeline.Execute(ctx, action, handler)

	// Write response back to client
	if err != nil {
		http.Error(w, fmt.Sprintf("pipeline error: %v", err), http.StatusInternalServerError)
		return
	}

	if result.Error != nil {
		http.Error(w, result.Error.Error(), http.StatusInternalServerError)
		return
	}

	// Return the output as JSON (same as upstream)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result.Output)
}

// CredentialManager handles encrypted credential storage and retrieval.
type CredentialManager struct {
	pool *pgxpool.Pool
}

// NewCredentialManager creates a new credential manager.
func NewCredentialManager(pool *pgxpool.Pool) *CredentialManager {
	return &CredentialManager{pool: pool}
}

// GetCredential retrieves and decrypts a credential.
func (cm *CredentialManager) GetCredential(ctx context.Context, workspaceID, connector string) (string, error) {
	var encrypted []byte
	err := cm.pool.QueryRow(ctx, `
		SELECT encrypted FROM credentials
		WHERE workspace_id = $1 AND connector = $2
		LIMIT 1
	`, workspaceID, connector).Scan(&encrypted)

	if err != nil {
		return "", fmt.Errorf("credential not found: %w", err)
	}

	// Decrypt using workspace ID as AAD
	aesKey, err := crypto.KeyFromHex(workspaceID) // In production, use proper key derivation
	if err != nil {
		return "", fmt.Errorf("derive key: %w", err)
	}

	aes, err := crypto.NewAES(aesKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	decrypted, err := aes.Decrypt(encrypted, []byte(workspaceID))
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(decrypted), nil
}

// StoreCredential encrypts and stores a credential.
func (cm *CredentialManager) StoreCredential(ctx context.Context, workspaceID, connector, label, apiKey string) error {
	keyHash := crypto.Hash(apiKey)

	// Encrypt using workspace ID as AAD
	aesKey, err := crypto.KeyFromHex(workspaceID)
	if err != nil {
		return fmt.Errorf("derive key: %w", err)
	}

	aes, err := crypto.NewAES(aesKey)
	if err != nil {
		return fmt.Errorf("create cipher: %w", err)
	}

	encrypted, err := aes.Encrypt([]byte(apiKey), []byte(workspaceID))
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	_, err = cm.pool.Exec(ctx, `
		INSERT INTO credentials (workspace_id, connector, label, key_hash, encrypted)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (workspace_id, connector, key_hash) DO NOTHING
	`, workspaceID, connector, label, keyHash, encrypted)

	return err
}
