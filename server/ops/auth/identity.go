package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kave-io/kave/core/pkg/authhash"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/authctx"
)

var (
	ErrTokensDisabled  = errors.New("auth tokens disabled")
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrInvalidBearer   = errors.New("invalid authorization header")
)

// ParseIdentity resolves an Authorization header into an Identity.
func ParseIdentity(ctx context.Context, authorization string, app store.AppStore, tokens *TokenManager, allowLegacy bool) (authctx.Identity, error) {
	if strings.TrimSpace(authorization) == "" {
		return authctx.Identity{Kind: authctx.KindAnonymous}, nil
	}

	token, ok := strings.CutPrefix(strings.TrimSpace(authorization), "Bearer ")
	if !ok {
		return authctx.Identity{Kind: authctx.KindInvalid, RawAuthorization: authorization, Err: ErrInvalidBearer.Error()}, ErrInvalidBearer
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return authctx.Identity{Kind: authctx.KindInvalid, RawAuthorization: authorization, Err: ErrInvalidBearer.Error()}, ErrInvalidBearer
	}

	if strings.HasPrefix(token, "kav_") {
		trimmed := strings.TrimPrefix(token, "kav_")
		if tokens != nil && tokens.Enabled() {
			if id, err := tokens.Verify(trimmed); err == nil {
				id.RawAuthorization = authorization
				return *id, nil
			}
		}
		if allowLegacy {
			if id, err := legacyAgentIdentity(ctx, app, token, authorization); err == nil {
				return id, nil
			}
		}
		return authctx.Identity{Kind: authctx.KindInvalid, RawAuthorization: authorization, Err: ErrUnauthenticated.Error()}, ErrUnauthenticated
	}

	if tokens != nil && tokens.Enabled() {
		if id, err := tokens.Verify(token); err == nil {
			id.RawAuthorization = authorization
			return *id, nil
		}
	}

	if allowLegacy {
		if id, err := legacyIdentity(ctx, app, token, authorization); err == nil {
			return id, nil
		}
	}

	return authctx.Identity{Kind: authctx.KindInvalid, RawAuthorization: authorization, Err: ErrUnauthenticated.Error()}, ErrUnauthenticated
}

func legacyIdentity(ctx context.Context, app store.AppStore, token, authorization string) (authctx.Identity, error) {
	if app == nil {
		return authctx.Identity{}, ErrUnauthenticated
	}

	if sess, err := app.GetSessionByHash(ctx, string(authhash.HashToken(token))); err == nil && sess != nil {
		if sess.RevokedAt != nil {
			return authctx.Identity{}, ErrUnauthenticated
		}
		if sess.ExpiresAt > 0 && sess.ExpiresAt <= nowMS() {
			return authctx.Identity{}, ErrUnauthenticated
		}
		return authctx.Identity{
			Kind:             authctx.KindUser,
			OrgID:            sess.OrgID,
			UserID:           sess.UserID,
			SessionID:        sess.ID,
			TokenID:          sess.ID,
			RawAuthorization: authorization,
			Legacy:           true,
		}, nil
	}

	if tok, err := app.GetAgentTokenByHash(ctx, string(authhash.HashToken(token))); err == nil && tok != nil {
		if tok.RevokedAt != nil {
			return authctx.Identity{}, ErrUnauthenticated
		}
		if tok.ExpiresAt > 0 && tok.ExpiresAt <= nowMS() {
			return authctx.Identity{}, ErrUnauthenticated
		}
		return authctx.Identity{
			Kind:             authctx.KindAgent,
			OrgID:            tok.OrgID,
			AgentID:          tok.AgentID,
			ProjectID:        tok.ProjectID,
			TokenID:          tok.ID,
			Connectors:       append([]string(nil), tok.Connectors...),
			Methods:          append([]string(nil), tok.Methods...),
			Scopes:           append([]string(nil), tok.Scopes...),
			RawAuthorization: authorization,
			Legacy:           true,
		}, nil
	}

	if tok, err := app.GetAPITokenByHash(ctx, string(authhash.HashToken(token))); err == nil && tok != nil {
		if tok.RevokedAt != nil {
			return authctx.Identity{}, ErrUnauthenticated
		}
		if tok.ExpiresAt != nil && *tok.ExpiresAt <= nowMS() {
			return authctx.Identity{}, ErrUnauthenticated
		}
		return authctx.Identity{
			Kind:             authctx.KindUser,
			OrgID:            tok.OrgID,
			UserID:           tok.UserID,
			TokenID:          tok.ID,
			Scopes:           append([]string(nil), tok.Scopes...),
			RawAuthorization: authorization,
			Legacy:           true,
		}, nil
	}

	return authctx.Identity{}, ErrUnauthenticated
}

func legacyAgentIdentity(ctx context.Context, app store.AppStore, token, authorization string) (authctx.Identity, error) {
	if app == nil {
		return authctx.Identity{}, ErrUnauthenticated
	}
	if tok, err := app.GetAgentTokenByHash(ctx, string(authhash.HashToken(token))); err == nil && tok != nil {
		if tok.RevokedAt != nil {
			return authctx.Identity{}, ErrUnauthenticated
		}
		if tok.ExpiresAt > 0 && tok.ExpiresAt <= nowMS() {
			return authctx.Identity{}, ErrUnauthenticated
		}
		return authctx.Identity{
			Kind:             authctx.KindAgent,
			OrgID:            tok.OrgID,
			AgentID:          tok.AgentID,
			ProjectID:        tok.ProjectID,
			TokenID:          tok.ID,
			Connectors:       append([]string(nil), tok.Connectors...),
			Methods:          append([]string(nil), tok.Methods...),
			Scopes:           append([]string(nil), tok.Scopes...),
			RawAuthorization: authorization,
			Legacy:           true,
		}, nil
	}
	return authctx.Identity{}, ErrUnauthenticated
}

func nowMS() int64 {
	return timeNowMS()
}

// timeNowMS is isolated for tests.
var timeNowMS = func() int64 {
	return time.Now().UnixMilli()
}
