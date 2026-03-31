package paseto

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	paseto "aidanwoods.dev/go-paseto"
	"github.com/google/uuid"
)

type Config struct {
	Mode Mode

	Issuer   string
	Audience string

	AccessTTL  time.Duration
	RefreshTTL time.Duration

	Implicit []byte
}

type Manager struct {
	cfg   Config
	keys  Keys
	parse paseto.Parser
}

func New(cfg Config, keys Keys) (*Manager, error) {
	if cfg.Mode != keys.Mode {
		return nil, ErrConfig{Msg: "cfg.Mode must match keys.Mode"}
	}
	if cfg.Issuer == "" {
		return nil, ErrConfig{Msg: "Issuer is required"}
	}
	if cfg.Audience == "" {
		return nil, ErrConfig{Msg: "Audience is required"}
	}
	if cfg.AccessTTL <= 0 {
		cfg.AccessTTL = 15 * time.Minute
	}
	if cfg.RefreshTTL <= 0 {
		cfg.RefreshTTL = 30 * 24 * time.Hour
	}

	p := paseto.NewParser()
	p.AddRule(paseto.IssuedBy(cfg.Issuer))
	p.AddRule(paseto.ForAudience(cfg.Audience))
	p.AddRule(paseto.NotExpired())
	p.AddRule(paseto.ValidAt(time.Now()))

	return &Manager{cfg: cfg, keys: keys, parse: p}, nil
}

// IssueAccess issues an access token. scope should be "FULL" for normal access
// or "REGISTRATION_COMPLETION" for first-login provisioned users.
// An empty scope is treated the same as "FULL".
func (m *Manager) IssueAccess(userID uuid.UUID, sessionID *uuid.UUID, scope string) (string, error) {
	return m.issueWithScope(TokenTypeAccess, userID, sessionID, scope, m.cfg.AccessTTL)
}

func (m *Manager) IssueRefresh(userID uuid.UUID, sessionID *uuid.UUID) (string, error) {
	return m.issueWithScope(TokenTypeRefresh, userID, sessionID, "", m.cfg.RefreshTTL)
}

func (m *Manager) Verify(tokenStr string) (*Claims, error) {
	var (
		tok *paseto.Token
		err error
	)

	switch m.cfg.Mode {
	case ModeLocal:
		if m.keys.Symmetric == nil {
			return nil, ErrConfig{Msg: "missing symmetric key"}
		}
		tok, err = m.parse.ParseV4Local(*m.keys.Symmetric, tokenStr, m.cfg.Implicit)
	case ModePublic:
		if m.keys.Public == nil {
			return nil, ErrConfig{Msg: "missing public key"}
		}
		tok, err = m.parse.ParseV4Public(*m.keys.Public, tokenStr, m.cfg.Implicit)
	default:
		return nil, ErrConfig{Msg: "unknown mode"}
	}

	if err != nil {
		return nil, ErrInvalidToken{Err: err}
	}

	claims, err := extractClaims(tok, m.cfg.Issuer, m.cfg.Audience)
	if err != nil {
		return nil, ErrInvalidToken{Err: err}
	}

	return claims, nil
}

func (m *Manager) issueWithScope(tt TokenType, userID uuid.UUID, sessionID *uuid.UUID, scope string, ttl time.Duration) (string, error) {
	now := time.Now()

	tok := paseto.NewToken()
	tok.SetIssuer(m.cfg.Issuer)
	tok.SetAudience(m.cfg.Audience)

	jti := randHex(16)
	tok.SetJti(jti)

	tok.SetIssuedAt(now)
	tok.SetNotBefore(now)
	tok.SetExpiration(now.Add(ttl))

	// subject: default to user id
	tok.SetSubject(userID.String())

	// custom claims
	tok.SetString("typ", string(tt))
	tok.SetString("uid", userID.String())
	if sessionID != nil {
		tok.SetString("sid", sessionID.String())
	}
	if scope != "" && scope != "FULL" {
		tok.SetString("scp", scope)
	}

	switch m.cfg.Mode {
	case ModeLocal:
		if m.keys.Symmetric == nil {
			return "", ErrConfig{Msg: "missing symmetric key"}
		}
		return tok.V4Encrypt(*m.keys.Symmetric, m.cfg.Implicit), nil

	case ModePublic:
		if m.keys.Secret == nil {
			return "", ErrConfig{Msg: "missing secret key"}
		}
		return tok.V4Sign(*m.keys.Secret, m.cfg.Implicit), nil

	default:
		return "", ErrConfig{Msg: "unknown mode"}
	}
}

func randHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func extractClaims(tok *paseto.Token, iss, aud string) (*Claims, error) {
	// Standard claims
	jti, err := tok.GetJti()
	if err != nil {
		return nil, err
	}

	sub, err := tok.GetSubject()
	if err != nil {
		return nil, err
	}

	iat, err := tok.GetIssuedAt()
	if err != nil {
		return nil, err
	}

	nbf, err := tok.GetNotBefore()
	if err != nil {
		return nil, err
	}

	exp, err := tok.GetExpiration()
	if err != nil {
		return nil, err
	}

	out := &Claims{
		Issuer:      iss,
		Audience:    aud,
		TokenID:     jti,
		Subject:     sub,
		IssuedAt:    iat,
		NotBefore:   nbf,
		ExpiresAt:   exp,
		RawFooter:   tok.Footer(),
		RawClaimsJS: tok.ClaimsJSON(),
	}

	// Custom claims
	typ, err := tok.GetString("typ")
	if err != nil {
		return nil, err
	}
	out.Type = TokenType(typ)

	uidStr, err := tok.GetString("uid")
	if err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(uidStr)
	if err != nil {
		return nil, err
	}
	out.UserID = uid

	// sid is optional
	if sidStr, err := tok.GetString("sid"); err == nil {
		if sid, err := uuid.Parse(sidStr); err == nil {
			out.SessionID = &sid
		} else {
			return nil, err
		}
	}

	// scp is optional; absent means FULL
	if scp, err := tok.GetString("scp"); err == nil {
		out.Scope = scp
	}

	return out, nil
}
