// Package httpapi owns the single-listener HTTP/Connect boundary for V2.
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	connect "connectrpc.com/connect"
	corev2 "github.com/kave-io/kave/core/v2"
	v2authctx "github.com/kave-io/kave/server/internal/v2/authctx"
	v2postgres "github.com/kave-io/kave/server/internal/v2/postgres"
)

type databaseServiceKeyAuthenticator interface {
	AuthenticateRaw(context.Context, string) (v2postgres.ServiceKeyIdentity, error)
}

// Identity is the non-secret authentication result accepted by the HTTP
// boundary. Keeping it local prevents handlers from depending on database
// records or seeing raw credentials.
type Identity struct {
	AccountID       corev2.Ref
	NamespaceID     corev2.Ref
	ServiceKeyID    corev2.Ref
	Operations      []corev2.Operation
	AllowedAgentIDs []corev2.Ref
	CanAssertScope  bool
}

type Authenticator interface {
	AuthenticateRaw(context.Context, string) (Identity, error)
}

// DatabaseAuthenticator adapts namespace-bound Postgres service keys to the
// transport identity.
type DatabaseAuthenticator struct {
	database databaseServiceKeyAuthenticator
}

func NewDatabaseAuthenticator(database databaseServiceKeyAuthenticator) *DatabaseAuthenticator {
	return &DatabaseAuthenticator{database: database}
}

func (a *DatabaseAuthenticator) AuthenticateRaw(ctx context.Context, rawKey string) (Identity, error) {
	if a == nil || a.database == nil {
		return Identity{}, v2postgres.ErrNilPool
	}
	record, err := a.database.AuthenticateRaw(ctx, rawKey)
	if err != nil {
		return Identity{}, err
	}
	operations := make([]corev2.Operation, 0, len(record.Capabilities))
	for _, capability := range record.Capabilities {
		switch operation := corev2.Operation(capability); operation {
		case corev2.OperationConfigApply, corev2.OperationSecretsWrite,
			corev2.OperationKeysManage, corev2.OperationLimitsSync,
			corev2.OperationUsageRead, corev2.OperationAuditRead,
			corev2.OperationConsume, corev2.OperationInvoke:
			operations = append(operations, operation)
		}
	}
	agentIDs := make([]corev2.Ref, 0, len(record.AllowedAgentIDs))
	for _, agentID := range record.AllowedAgentIDs {
		agentIDs = append(agentIDs, corev2.Ref(agentID))
	}
	return Identity{
		AccountID:       corev2.Ref(record.AccountID),
		NamespaceID:     corev2.Ref(record.NamespaceID),
		ServiceKeyID:    corev2.Ref(record.ServiceKeyID),
		Operations:      operations,
		AllowedAgentIDs: agentIDs,
		CanAssertScope:  record.CanAssertScope,
	}, nil
}

// AuthMiddleware rejects malformed, missing, revoked, expired, and unknown
// service keys before any V2 handler runs.
type AuthMiddleware struct {
	authenticator Authenticator
	errorWriter   *connect.ErrorWriter
}

func NewAuthMiddleware(authenticator Authenticator) *AuthMiddleware {
	return &AuthMiddleware{authenticator: authenticator, errorWriter: connect.NewErrorWriter()}
}

func (m *AuthMiddleware) WrapConnect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, err := m.authenticate(r)
		if err != nil {
			_ = m.errorWriter.Write(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(v2authctx.WithCaller(r.Context(), caller)))
	})
}

func (m *AuthMiddleware) authenticate(r *http.Request) (corev2.Caller, error) {
	if m == nil || m.authenticator == nil || r == nil {
		return corev2.Caller{}, connect.NewError(connect.CodeUnavailable, errors.New("authentication unavailable"))
	}
	values := r.Header.Values("Authorization")
	if len(values) != 1 {
		return corev2.Caller{}, unauthenticated()
	}
	rawKey, ok := strings.CutPrefix(values[0], "Bearer ")
	if !ok || rawKey == "" || strings.TrimSpace(rawKey) != rawKey {
		return corev2.Caller{}, unauthenticated()
	}

	identity, err := m.authenticator.AuthenticateRaw(r.Context(), rawKey)
	if errors.Is(err, v2postgres.ErrInvalidServiceKey) {
		return corev2.Caller{}, unauthenticated()
	}
	if err != nil {
		return corev2.Caller{}, connect.NewError(connect.CodeUnavailable, errors.New("authentication unavailable"))
	}

	return corev2.Caller{
		AccountID:       identity.AccountID,
		NamespaceID:     identity.NamespaceID,
		ServiceKeyID:    identity.ServiceKeyID,
		Operations:      identity.Operations,
		AllowedAgentIDs: identity.AllowedAgentIDs,
		CanAssertScope:  identity.CanAssertScope,
	}, nil
}

func unauthenticated() error {
	return connect.NewError(connect.CodeUnauthenticated, errors.New("invalid service key"))
}
