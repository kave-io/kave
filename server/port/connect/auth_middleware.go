package connectport

import (
	"net/http"

	connect "connectrpc.com/connect"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/authctx"
	serverauth "github.com/kave-io/kave/server/ops/auth"
)

// AuthConfig defines the authentication boundary for the Connect surface.
// Every mounted procedure is private; public authentication procedures live on
// the gRPC surface until they are deliberately migrated.
type AuthConfig struct {
	App            store.AppStore
	Tokens         *serverauth.TokenManager
	AllowAnonymous bool
	AllowLegacy    bool
}

// NewAuthMiddleware authenticates before a Connect handler is entered. Invalid
// credentials are never represented as a context identity: the request is
// terminated with a protocol-correct unauthenticated error at the boundary.
func NewAuthMiddleware(cfg AuthConfig) func(http.Handler) http.Handler {
	errorWriter := connect.NewErrorWriter()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authorization := r.Header.Values("Authorization")
			if len(authorization) > 1 {
				connectErr := connect.NewError(connect.CodeUnauthenticated, serverauth.ErrUnauthenticated)
				_ = errorWriter.Write(w, r, connectErr)
				return
			}
			rawAuthorization := ""
			if len(authorization) == 1 {
				rawAuthorization = authorization[0]
			}
			id, err := serverauth.ParseIdentity(
				r.Context(),
				rawAuthorization,
				cfg.App,
				cfg.Tokens,
				cfg.AllowLegacy,
			)
			if err != nil || id.IsInvalid() || (id.IsAnonymous() && !cfg.AllowAnonymous) {
				connectErr := connect.NewError(connect.CodeUnauthenticated, serverauth.ErrUnauthenticated)
				_ = errorWriter.Write(w, r, connectErr)
				return
			}

			next.ServeHTTP(w, r.WithContext(authctx.With(r.Context(), id)))
		})
	}
}
