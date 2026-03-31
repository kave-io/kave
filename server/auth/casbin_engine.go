package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/auth"
	"github.com/kave-io/kave/core/intercept"
	infraAuth "github.com/kave-io/kave/server/infra/casbin"
	infraPaseto "github.com/kave-io/kave/server/infra/paseto"
)

// CasbinPolicyEngine implements core/auth.PolicyEngine using Casbin + PASETO.
// It enforces agent permissions and issues/revokes access tokens.
type CasbinPolicyEngine struct {
	pool    *pgxpool.Pool
	casbin  infraAuth.Casbin
	paseto  *infraPaseto.Manager
}

// New creates a new CasbinPolicyEngine.
func New(pool *pgxpool.Pool, casbin infraAuth.Casbin, paseto *infraPaseto.Manager) *CasbinPolicyEngine {
	return &CasbinPolicyEngine{
		pool:   pool,
		casbin: casbin,
		paseto: paseto,
	}
}

// Allow checks if an agent is allowed to perform an action.
// Extracts agent ID from context (set by auth middleware) and checks against Casbin policies.
func (e *CasbinPolicyEngine) Allow(ctx context.Context, agentID string, action *intercept.Action) (bool, error) {
	// Check if token is revoked
	revoked, err := e.isTokenRevoked(ctx, agentID)
	if err != nil {
		return false, fmt.Errorf("check token revocation: %w", err)
	}
	if revoked {
		return false, nil
	}

	// Check Casbin policy: agent can call this connector/method
	// For now, use a simple model: allow if agent is assigned a policy that permits the connector/method
	// The Casbin model handles wildcard matching (* for all connectors/methods)
	allowed, err := e.casbin.Enforce(ctx,
		infraAuth.GroupSubject(agentID),
		infraAuth.Domain("agent"),
		infraAuth.Resource(action.Connector),
		infraAuth.Action(action.Method),
	)

	if err != nil {
		return false, fmt.Errorf("casbin enforce: %w", err)
	}

	return allowed, nil
}

// IssueToken creates a new PASETO token for an agent.
// The token encodes the agent ID, allowed connectors, allowed methods, and budget cap.
func (e *CasbinPolicyEngine) IssueToken(ctx context.Context, agentID string, scope auth.Scope) (auth.Token, error) {
	// Generate token with agent claims
	// Note: we need to adapt the PASETO manager to issue agent tokens instead of user tokens
	// For now, use a simple approach: store the token in revoked_tokens table with a unique ID

	// Generate unique token ID
	h := sha256.Sum256([]byte(agentID + time.Now().String()))
	tokenID := base32.StdEncoding.EncodeToString(h[:])[:16]

	// Insert into tokens table (create table in migration)
	_, err := e.pool.Exec(ctx, `
		INSERT INTO agent_tokens (id, agent_id, connectors, methods, budget_cap_usd, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`, tokenID, agentID, scope.AllowedConnectors, scope.AllowedMethods, scope.BudgetCapUSD, scope.ExpiresAt)

	if err != nil {
		return auth.Token{}, fmt.Errorf("insert token: %w", err)
	}

	return auth.Token{
		ID:        tokenID,
		AgentID:   agentID,
		Raw:       tokenID, // In a real implementation, this would be a PASETO token string
		Scope:     scope,
		IssuedAt:  time.Now(),
		ExpiresAt: scope.ExpiresAt,
	}, nil
}

// RevokeToken invalidates a token by adding it to the revoked list.
func (e *CasbinPolicyEngine) RevokeToken(ctx context.Context, tokenID string) error {
	_, err := e.pool.Exec(ctx, `
		INSERT INTO revoked_tokens (token_id, revoked_at)
		VALUES ($1, NOW())
		ON CONFLICT DO NOTHING
	`, tokenID)

	return err
}

// isTokenRevoked checks if a token has been revoked.
func (e *CasbinPolicyEngine) isTokenRevoked(ctx context.Context, tokenID string) (bool, error) {
	var count int
	err := e.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM revoked_tokens WHERE token_id = $1
	`, tokenID).Scan(&count)

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Implement Interceptor interface for pipeline integration

// Before checks if the action is allowed.
func (e *CasbinPolicyEngine) Before(ctx context.Context, action *intercept.Action) (*intercept.Action, error) {
	// Extract agent ID from action metadata or context
	// For now, we'll get it from the run's agent_id via database lookup
	var agentID string
	err := e.pool.QueryRow(ctx, `SELECT agent_id FROM runs WHERE id = $1`, action.RunID).Scan(&agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent from run: %w", err)
	}

	allowed, err := e.Allow(ctx, agentID, action)
	if err != nil {
		return nil, fmt.Errorf("allow check: %w", err)
	}

	if !allowed {
		return nil, fmt.Errorf("action denied: agent %s cannot call %s.%s", agentID, action.Connector, action.Method)
	}

	return action, nil
}

// After is a no-op for auth.
func (e *CasbinPolicyEngine) After(ctx context.Context, action *intercept.Action, result *intercept.Result) error {
	return nil
}

// Name returns the interceptor name.
func (e *CasbinPolicyEngine) Name() string {
	return "auth"
}
