package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	paseto "aidanwoods.dev/go-paseto"
	"github.com/kave-io/kave/server/internal/authctx"
)

const (
	tokenIssuer   = "kave"
	tokenAudience = "kave-api"
)

// TokenManager issues and verifies PASETO-backed auth tokens.
//
// When no symmetric key is configured, it falls back to opaque tokens so the
// server remains usable in dev-only configurations.
type TokenManager struct {
	symmetric  *paseto.V4SymmetricKey
	issuer     string
	audience   string
	sessionTTL time.Duration
	tokenTTL   time.Duration
}

func NewTokenManager(keyHex string, sessionTTL, tokenTTL time.Duration) (*TokenManager, error) {
	m := &TokenManager{
		issuer:     tokenIssuer,
		audience:   tokenAudience,
		sessionTTL: sessionTTL,
		tokenTTL:   tokenTTL,
	}
	if m.sessionTTL <= 0 {
		m.sessionTTL = 24 * time.Hour
	}
	if m.tokenTTL <= 0 {
		m.tokenTTL = 30 * 24 * time.Hour
	}
	if keyHex == "" {
		return m, nil
	}
	key, err := paseto.V4SymmetricKeyFromHex(keyHex)
	if err != nil {
		return nil, fmt.Errorf("auth token key: %w", err)
	}
	m.symmetric = &key
	return m, nil
}

func (m *TokenManager) Enabled() bool { return m != nil && m.symmetric != nil }

func (m *TokenManager) IssueSession(userID, sessionID, orgID string) (string, error) {
	if !m.Enabled() {
		return opaqueToken("ks_"), nil
	}
	return m.issue("session", userID, sessionID, orgID, "", "", "", m.sessionTTL)
}

func (m *TokenManager) IssueAgentToken(agentID, projectID, envID, orgID string) (string, error) {
	if !m.Enabled() {
		return opaqueToken("kav_"), nil
	}
	return m.issue("agent", agentID, "", orgID, agentID, projectID, envID, m.tokenTTL)
}

func (m *TokenManager) Verify(token string) (*authctx.Identity, error) {
	if !m.Enabled() {
		return nil, ErrTokensDisabled
	}
	parser := paseto.NewParser()
	parser.AddRule(paseto.IssuedBy(m.issuer))
	parser.AddRule(paseto.ForAudience(m.audience))
	parser.AddRule(paseto.NotExpired())
	parser.AddRule(paseto.ValidAt(time.Now()))
	tok, err := parser.ParseV4Local(*m.symmetric, token, nil)
	if err != nil {
		return nil, err
	}
	kind, err := tok.GetString("kind")
	if err != nil {
		return nil, err
	}
	id := authctx.Identity{
		Kind:    authctx.Kind(kind),
		TokenID: must(tok.GetJti()),
		OrgID:   must(tok.GetString("org")),
	}
	switch authctx.Kind(kind) {
	case authctx.KindUser:
		id.UserID = must(tok.GetString("uid"))
		id.SessionID = must(tok.GetString("sid"))
	case authctx.KindAgent:
		id.AgentID = must(tok.GetString("aid"))
		id.ProjectID = must(tok.GetString("pid"))
		id.EnvID = must(tok.GetString("eid"))
	default:
		return nil, fmt.Errorf("unsupported token kind %q", kind)
	}
	return &id, nil
}

func (m *TokenManager) issue(kind, subject, sessionID, orgID, agentID, projectID, envID string, ttl time.Duration) (string, error) {
	now := time.Now()
	tok := paseto.NewToken()
	tok.SetIssuer(m.issuer)
	tok.SetAudience(m.audience)
	tok.SetJti(randHex(16))
	tok.SetIssuedAt(now)
	tok.SetNotBefore(now)
	tok.SetExpiration(now.Add(ttl))
	tok.SetSubject(subject)
	tok.SetString("kind", kind)
	tok.SetString("org", orgID)
	switch kind {
	case "session":
		tok.SetString("uid", subject)
		if sessionID != "" {
			tok.SetString("sid", sessionID)
		}
	case "agent":
		tok.SetString("aid", agentID)
		tok.SetString("pid", projectID)
		tok.SetString("eid", envID)
	default:
		return "", fmt.Errorf("unsupported token kind %q", kind)
	}
	return tok.V4Encrypt(*m.symmetric, nil), nil
}

func randHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func opaqueToken(prefix string) string {
	if prefix == "" {
		prefix = "kav_"
	}
	return prefix + randHex(24)
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
