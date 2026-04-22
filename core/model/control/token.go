package control

import "github.com/kave-io/kave/core/pkg/money"

// Session is a user -> daemon login session.
type Session struct {
	ID         string
	OrgID      string
	UserID     string
	TokenHash  []byte
	ExpiresAt  int64
	CreatedAt  int64
	LastUsedAt *int64
	UserAgent  string
	IP         string
	RevokedAt  *int64
}

// APIToken is a user-issued machine token.
type APIToken struct {
	ID         string
	OrgID      string
	UserID     string
	Name       string
	TokenHash  []byte
	Scopes     []string
	ExpiresAt  *int64
	LastUsedAt *int64
	RevokedAt  *int64
	CreatedAt  int64
}

// AgentToken is an authorization token issued to an agent process.
type AgentToken struct {
	ID        string
	OrgID     string
	AgentID   string
	ProjectID string // legacy compatibility

	// UX / identification
	Name        string
	Description string
	TokenPrefix string
	Hash        string
	TokenHash   []byte
	IssuedFor   string
	IssuedBy    string

	// Scope
	Connectors []string
	Methods    []string
	Scopes     []string
	BudgetCap  *money.Amount

	// Validity / usage
	NotBefore    int64
	ExpiresAt    int64
	LastUsedAt   *int64
	RevokedAt    *int64
	RevokedBy    string
	RevokeReason string
	CreatedAt    int64
}

// Role stores a set of Casbin permissions in Kave.
type Role struct {
	ID          string
	OrgID       string
	Name        string
	Permissions []string
	CreatedAt   int64
	UpdatedAt   int64
}

// Binding assigns a subject to a role within an optional scope.
type Binding struct {
	ID        string
	OrgID     string
	RoleID    string
	Subject   string
	Scope     string
	CreatedAt int64
}
