package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	coreauth "github.com/kave-io/kave/core/auth"
	"github.com/kave-io/kave/core/intercept"
	infraAuth "github.com/kave-io/kave/server/infra/casbin"
	infraPaseto "github.com/kave-io/kave/server/infra/paseto"
)

// Scope defines what an agent token is allowed to do.
type Scope struct {
	AllowedConnectors []string
	AllowedMethods    []string
	BudgetCapUSD      *float64
	ExpiresAt         time.Time
}

// Token is an issued agent access token.
type Token struct {
	ID        string
	AgentID   string
	Raw       string
	Scope     Scope
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// CasbinPolicyEngine implements core/auth.PolicyEngine and intercept.Interceptor.
// It enforces agent permissions via Casbin and issues/revokes PASETO tokens.
type CasbinPolicyEngine struct {
	pool   *pgxpool.Pool
	casbin infraAuth.Casbin
	paseto *infraPaseto.Manager
}

// New creates a new CasbinPolicyEngine.
func New(pool *pgxpool.Pool, casbin infraAuth.Casbin, paseto *infraPaseto.Manager) *CasbinPolicyEngine {
	return &CasbinPolicyEngine{
		pool:   pool,
		casbin: casbin,
		paseto: paseto,
	}
}

// Allowed implements core/auth.PolicyEngine.
// Checks policy AllowedTypes/AllowedConnectors/AllowedMethods.
func (e *CasbinPolicyEngine) Allowed(_ context.Context, actionType intercept.ActionType, connector, method string, policy *intercept.Policy) (bool, error) {
	if !matchesAny(policy.AllowedTypes, string(actionType)) {
		return false, nil
	}
	if !matchesAny(policy.AllowedConnectors, connector) {
		return false, nil
	}
	if !matchesAny(policy.AllowedMethods, method) {
		return false, nil
	}
	return true, nil
}

// matchesAny returns true if list contains "*" or the target value.
func matchesAny(list []string, target string) bool {
	for _, v := range list {
		if v == "*" || v == target {
			return true
		}
	}
	return false
}

// Allow checks Casbin rules for a specific agent+action combination.
func (e *CasbinPolicyEngine) Allow(ctx context.Context, agentID string, action *intercept.Action) (bool, error) {
	revoked, err := e.isTokenRevoked(ctx, agentID)
	if err != nil {
		return false, fmt.Errorf("check token revocation: %w", err)
	}
	if revoked {
		return false, nil
	}

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

// IssueToken creates a new access token for an agent.
func (e *CasbinPolicyEngine) IssueToken(ctx context.Context, agentID string, scope Scope) (Token, error) {
	h := sha256.Sum256([]byte(agentID + time.Now().String()))
	tokenID := base32.StdEncoding.EncodeToString(h[:])[:16]

	_, err := e.pool.Exec(ctx, `
		INSERT INTO agent_tokens (id, agent_id, connectors, methods, budget_cap_usd, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`, tokenID, agentID, scope.AllowedConnectors, scope.AllowedMethods, scope.BudgetCapUSD, scope.ExpiresAt)
	if err != nil {
		return Token{}, fmt.Errorf("insert token: %w", err)
	}

	return Token{
		ID:        tokenID,
		AgentID:   agentID,
		Raw:       tokenID,
		Scope:     scope,
		IssuedAt:  time.Now(),
		ExpiresAt: scope.ExpiresAt,
	}, nil
}

// RevokeToken invalidates a token.
func (e *CasbinPolicyEngine) RevokeToken(ctx context.Context, tokenID string) error {
	_, err := e.pool.Exec(ctx, `
		INSERT INTO revoked_tokens (token_id, revoked_at)
		VALUES ($1, NOW())
		ON CONFLICT DO NOTHING
	`, tokenID)
	return err
}

func (e *CasbinPolicyEngine) isTokenRevoked(ctx context.Context, tokenID string) (bool, error) {
	var count int
	err := e.pool.QueryRow(ctx, `SELECT COUNT(*) FROM revoked_tokens WHERE token_id = $1`, tokenID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Before checks if the action is allowed via Casbin.
func (e *CasbinPolicyEngine) Before(ctx context.Context, action *intercept.Action) (*intercept.Action, error) {
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
func (e *CasbinPolicyEngine) After(_ context.Context, _ *intercept.Action, _ *intercept.Result) error {
	return nil
}

// Name returns the interceptor name.
func (e *CasbinPolicyEngine) Name() string { return "auth" }

// Ensure interface compliance.
var _ coreauth.PolicyEngine = (*CasbinPolicyEngine)(nil)
