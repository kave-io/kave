package mappers

import (
	controlmodel "github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/pkg/money"
)

// TokenIssue represents token issuance input from control-plane handlers.
type TokenIssue struct {
	ID          string
	AgentID     string
	ProjectID   string
	Name        string
	Description string
	IssuedFor   string
	IssuedBy    string
	Connectors  []string
	Methods     []string
	BudgetCap   *money.Amount
	Scopes      []string
	NotBefore   int64
	ExpiresAt   int64
	CreatedAt   *int64
}

// AgentTokenView is the response-safe shape for a token (shows prefix, not raw).
type AgentTokenView struct {
	ID          string
	AgentID     string
	ProjectID   string
	Name        string
	Description string
	TokenPrefix string
	IssuedFor   string
	IssuedBy    string
	Connectors  []string
	Methods     []string
	BudgetCapUSD *float64
	Scopes      []string
	NotBefore   int64
	ExpiresAt   int64
	LastUsedAt  *int64
	RevokedAt   *int64
	RevokedBy   string
	RevokeReason string
	CreatedAt   int64
}

// TokenIssueToModel converts token issue input to controlmodel.AgentToken.
func TokenIssueToModel(issue *TokenIssue) *controlmodel.AgentToken {
	if issue == nil {
		return nil
	}

	createdAt := msSinceEpoch()
	if issue.CreatedAt != nil {
		createdAt = *issue.CreatedAt
	}

	return &controlmodel.AgentToken{
		ID:          issue.ID,
		AgentID:     issue.AgentID,
		ProjectID:   issue.ProjectID,
		Name:        issue.Name,
		Description: issue.Description,
		IssuedFor:   issue.IssuedFor,
		IssuedBy:    issue.IssuedBy,
		Connectors:  issue.Connectors,
		Methods:     issue.Methods,
		BudgetCap:   issue.BudgetCap,
		Scopes:      issue.Scopes,
		NotBefore:   issue.NotBefore,
		ExpiresAt:   issue.ExpiresAt,
		CreatedAt:   createdAt,
	}
}

// AgentTokenToView converts persisted token data to response-safe view (prefix only, no hash).
func AgentTokenToView(token *controlmodel.AgentToken) *AgentTokenView {
	if token == nil {
		return nil
	}
	var capUSD *float64
	if token.BudgetCap != nil {
		v := token.BudgetCap.Dollars()
		capUSD = &v
	}
	return &AgentTokenView{
		ID:           token.ID,
		AgentID:      token.AgentID,
		ProjectID:    token.ProjectID,
		Name:         token.Name,
		Description:  token.Description,
		TokenPrefix:  token.TokenPrefix,
		IssuedFor:    token.IssuedFor,
		IssuedBy:     token.IssuedBy,
		Connectors:   token.Connectors,
		Methods:      token.Methods,
		BudgetCapUSD: capUSD,
		Scopes:       token.Scopes,
		NotBefore:    token.NotBefore,
		ExpiresAt:    token.ExpiresAt,
		LastUsedAt:   token.LastUsedAt,
		RevokedAt:    token.RevokedAt,
		RevokedBy:    token.RevokedBy,
		RevokeReason: token.RevokeReason,
		CreatedAt:    token.CreatedAt,
	}
}
