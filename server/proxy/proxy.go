package proxy

import (
	"context"
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

	// Read body as raw bytes; stored as Action.Input and forwarded upstream
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
		r.Body = io.NopCloser(strings.NewReader(string(body)))
	}

	action := &intercept.Action{
		Unit: intercept.Unit{
			ID:        uuid.New().String(),
			RunID:     agentID,
			Type:      intercept.TypeLLM,
			Connector: connector,
			Method:    method,
			Input:     body,
		},
		Status: intercept.StatusPending,
	}

	handler := func(ctx context.Context, action *intercept.Action) (*intercept.Result, error) {
		resp, err := p.upstream.Forward(r, connector, cred)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		tokenData := ExtractTokenUsage(respBody, connector)
		var tokenUsage *intercept.TokenUsage
		if len(tokenData) > 0 {
			tokenUsage = &intercept.TokenUsage{}
			if v, ok := tokenData["InputTokens"]; ok {
				tokenUsage.InputTokens = v.(int)
			}
			if v, ok := tokenData["OutputTokens"]; ok {
				tokenUsage.OutputTokens = v.(int)
			}
			if v, ok := tokenData["CacheRead"]; ok {
				tokenUsage.CacheRead = v.(int)
			}
			if v, ok := tokenData["CacheWrite"]; ok {
				tokenUsage.CacheWrite = v.(int)
			}
			if v, ok := tokenData["Model"]; ok {
				tokenUsage.Model = v.(string)
			}
		}

		return &intercept.Result{Body: respBody, TokenUsage: tokenUsage}, nil
	}

	result, err := p.pipeline.Execute(ctx, action, handler)
	if err != nil {
		http.Error(w, fmt.Sprintf("pipeline error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(result.Body)
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
