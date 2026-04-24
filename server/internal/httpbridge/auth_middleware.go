package httpbridge

import (
	"net/http"

	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/authctx"
	serverauth "github.com/kave-io/kave/server/ops/auth"
)

type AuthMiddleware struct {
	app            store.AppStore
	tokens         *serverauth.TokenManager
	allowLegacy    bool
	allowAnonymous bool
}

func NewAuthMiddleware(app store.AppStore, tokens *serverauth.TokenManager, allowAnonymous, allowLegacy bool) *AuthMiddleware {
	return &AuthMiddleware{
		app:            app,
		tokens:         tokens,
		allowLegacy:    allowLegacy,
		allowAnonymous: allowAnonymous,
	}
}

func (m *AuthMiddleware) Wrap(next http.Handler) http.Handler {
	if next == nil {
		next = http.DefaultServeMux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := serverauth.ParseIdentity(r.Context(), r.Header.Get("Authorization"), m.app, m.tokens, m.allowLegacy)
		if err == nil {
			if id.IsAnonymous() && !m.allowAnonymous {
				id.Kind = authctx.KindInvalid
				id.Err = serverauth.ErrUnauthenticated.Error()
			}
			r = r.WithContext(authctx.With(r.Context(), id))
		} else {
			r = r.WithContext(authctx.With(r.Context(), authctx.Identity{
				Kind:             authctx.KindInvalid,
				RawAuthorization: r.Header.Get("Authorization"),
				Err:              err.Error(),
			}))
		}
		next.ServeHTTP(w, r)
	})
}
