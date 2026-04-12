package control

import "github.com/kave-io/kave/core/pkg/money"

// AgentToken is an authorization token issued to an agent.
// Raw token is shown once at creation, then only the hash is stored.
type AgentToken struct {
	ID          string
	AgentID     string
	ProjectID   string // for index queries

	// UX / identification
	Name        string // human name; "prod CLI key"
	Description string
	TokenPrefix string // first 8 chars of raw token shown after creation for user identification
	Hash        string // SHA256(rawToken); used for lookup — raw token never persisted

	// Issuance
	IssuedFor string // "agent" | "service" | "cli" | "human" | "integration"
	IssuedBy  string // user ID or "system"

	// Scope
	Connectors []string
	Methods    []string
	BudgetCap  *money.Amount
	Scopes     []string // extension point for additional scope strings

	// Validity window
	NotBefore int64 // UnixMilli
	ExpiresAt int64 // UnixMilli; 0 = no expiry

	// Usage
	LastUsedAt *int64 // UnixMilli

	// Revocation (soft — for audit trail)
	RevokedAt    *int64 // UnixMilli; nil = active
	RevokedBy    string
	RevokeReason string

	CreatedAt int64 // UnixMilli
}

// AgentTokenUpdate holds partial update fields for a token (mainly revocation).
type AgentTokenUpdate struct {
	RevokedAt    *int64
	RevokedBy    *string
	RevokeReason *string
}
