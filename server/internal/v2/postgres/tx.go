package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const installScopeSQL = `
SELECT
    set_config('kave.account_id', $1, true),
    set_config('kave.namespace_id', $2, true)
`

var (
	ErrInvalidScope = errors.New("v2 postgres: invalid scope")
	ErrNilPool      = errors.New("v2 postgres: nil pool")
)

// Scope is the complete database isolation identity for one V2 request.
// Both values are required even for namespace creation: the desired namespace
// ID is known before its row is inserted.
type Scope struct {
	AccountID   string
	NamespaceID string
}

// Validate rejects values that cannot safely be installed as Postgres settings.
func (s Scope) Validate() error {
	for name, value := range map[string]string{
		"account_id":   s.AccountID,
		"namespace_id": s.NamespaceID,
	} {
		if value == "" || strings.TrimSpace(value) != value {
			return fmt.Errorf("%w: %s is required and must not have surrounding whitespace", ErrInvalidScope, name)
		}
		if len(value) > 255 {
			return fmt.Errorf("%w: %s exceeds 255 bytes", ErrInvalidScope, name)
		}
		if strings.IndexByte(value, 0) >= 0 {
			return fmt.Errorf("%w: %s contains NUL", ErrInvalidScope, name)
		}
	}
	return nil
}

// DBTX is the intentionally small query surface exposed inside a scoped
// transaction. Transaction lifecycle stays owned by ScopedRunner.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type transaction interface {
	DBTX
	Commit(context.Context) error
	Rollback(context.Context) error
}

type beginTxFunc func(context.Context, pgx.TxOptions) (transaction, error)

// ScopedRunner owns the V2 transaction boundary. It intentionally does not
// implement or embed the broad V1 AppStore interface.
type ScopedRunner struct {
	begin beginTxFunc
}

// NewScopedRunner creates a serializable, RLS-scoped transaction runner.
func NewScopedRunner(pool *pgxpool.Pool) (*ScopedRunner, error) {
	if pool == nil {
		return nil, ErrNilPool
	}
	return &ScopedRunner{
		begin: func(ctx context.Context, opts pgx.TxOptions) (transaction, error) {
			return pool.BeginTx(ctx, opts)
		},
	}, nil
}

// WithScope executes fn in a serializable transaction after installing both
// RLS settings with set_config(..., true). The true argument is essential: it
// makes the settings transaction-local so a pooled connection cannot leak one
// request's tenant identity into another request.
func (r *ScopedRunner) WithScope(ctx context.Context, scope Scope, fn func(context.Context, DBTX) error) error {
	return r.withScope(ctx, scope, pgx.Serializable, fn)
}

func (r *ScopedRunner) withScope(ctx context.Context, scope Scope, isolation pgx.TxIsoLevel, fn func(context.Context, DBTX) error) error {
	if r == nil || r.begin == nil {
		return ErrNilPool
	}
	if err := scope.Validate(); err != nil {
		return err
	}
	if fn == nil {
		return errors.New("v2 postgres: scoped transaction callback is nil")
	}

	tx, err := r.begin(ctx, pgx.TxOptions{IsoLevel: isolation})
	if err != nil {
		return fmt.Errorf("v2 postgres: begin scoped transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var installedAccount, installedNamespace string
	if err := tx.QueryRow(ctx, installScopeSQL, scope.AccountID, scope.NamespaceID).
		Scan(&installedAccount, &installedNamespace); err != nil {
		return fmt.Errorf("v2 postgres: install scope: %w", err)
	}
	if installedAccount != scope.AccountID || installedNamespace != scope.NamespaceID {
		return fmt.Errorf("v2 postgres: install scope: database returned unexpected scope")
	}

	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("v2 postgres: commit scoped transaction: %w", err)
	}
	return nil
}
