// Package paseto stays under server/internal/infra because its TTLs, claim
// shape, and issuance rules are server authentication policy, not a reusable
// generic token primitive.
package paseto

import (
	"time"

	"github.com/google/uuid"
)

type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// Claims is the app-facing token payload.
type Claims struct {
	Type TokenType

	UserID    uuid.UUID
	SessionID *uuid.UUID
	// Scope restricts what the token may access.
	// "FULL" (or empty) grants unrestricted access.
	// "REGISTRATION_COMPLETION" restricts to the completion endpoint only.
	Scope string

	Issuer   string
	Audience string

	IssuedAt    time.Time
	NotBefore   time.Time
	ExpiresAt   time.Time
	TokenID     string // jti
	Subject     string
	RawFooter   []byte
	RawClaimsJS []byte
}

// GetUserID implements authorize.ClaimsProvider and reqctx.AuthClaims interface.
func (c *Claims) GetUserID() uuid.UUID {
	return c.UserID
}

// GetSessionID implements reqctx.AuthClaims interface.
func (c *Claims) GetSessionID() *uuid.UUID {
	return c.SessionID
}

// GetTokenType implements reqctx.AuthClaims interface.
func (c *Claims) GetTokenType() string {
	return string(c.Type)
}

// IsExpired implements reqctx.AuthClaims interface.
func (c *Claims) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}
