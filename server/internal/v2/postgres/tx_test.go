package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type scanRow func(...any) error

func (r scanRow) Scan(dest ...any) error { return r(dest...) }

type fakeTransaction struct {
	queryRow  func(string, ...any) pgx.Row
	execs     []string
	commits   int
	rollbacks int
}

func (t *fakeTransaction) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	t.execs = append(t.execs, sql)
	return pgconn.NewCommandTag("OK"), nil
}

func (t *fakeTransaction) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unexpected Query call")
}

func (t *fakeTransaction) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	if t.queryRow == nil {
		return scanRow(func(...any) error { return errors.New("unexpected QueryRow call") })
	}
	return t.queryRow(sql, args...)
}

func (t *fakeTransaction) Commit(context.Context) error {
	t.commits++
	return nil
}

func (t *fakeTransaction) Rollback(context.Context) error {
	t.rollbacks++
	return nil
}

func TestScopeValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		scope Scope
		ok    bool
	}{
		{name: "valid", scope: Scope{AccountID: "account_01", NamespaceID: "namespace_01"}, ok: true},
		{name: "missing account", scope: Scope{NamespaceID: "namespace_01"}},
		{name: "missing namespace", scope: Scope{AccountID: "account_01"}},
		{name: "surrounding whitespace", scope: Scope{AccountID: " account_01", NamespaceID: "namespace_01"}},
		{name: "nul", scope: Scope{AccountID: "account_01", NamespaceID: "namespace\x00bad"}},
		{name: "too long", scope: Scope{AccountID: strings.Repeat("a", 256), NamespaceID: "namespace_01"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.scope.Validate()
			if tt.ok && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if !tt.ok && !errors.Is(err, ErrInvalidScope) {
				t.Fatalf("Validate() error = %v, want ErrInvalidScope", err)
			}
		})
	}
}

func TestScopedRunnerInstallsTransactionLocalScopeBeforeCallback(t *testing.T) {
	t.Parallel()

	scope := Scope{AccountID: "account_01", NamespaceID: "namespace_01"}
	tx := &fakeTransaction{}
	installed := false
	tx.queryRow = func(sql string, args ...any) pgx.Row {
		if !strings.Contains(sql, "set_config('kave.account_id', $1, true)") ||
			!strings.Contains(sql, "set_config('kave.namespace_id', $2, true)") {
			t.Fatalf("scope query is not transaction-local: %s", sql)
		}
		if len(args) != 2 || args[0] != scope.AccountID || args[1] != scope.NamespaceID {
			t.Fatalf("scope args = %#v", args)
		}
		return scanRow(func(dest ...any) error {
			*(dest[0].(*string)) = scope.AccountID
			*(dest[1].(*string)) = scope.NamespaceID
			installed = true
			return nil
		})
	}

	var gotOptions pgx.TxOptions
	runner := &ScopedRunner{begin: func(_ context.Context, options pgx.TxOptions) (transaction, error) {
		gotOptions = options
		return tx, nil
	}}
	err := runner.WithScope(context.Background(), scope, func(_ context.Context, got DBTX) error {
		if !installed {
			t.Fatal("callback ran before scope was installed")
		}
		if got != tx {
			t.Fatal("callback did not receive the scoped transaction")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithScope() error = %v", err)
	}
	if gotOptions.IsoLevel != pgx.Serializable {
		t.Fatalf("isolation = %q, want serializable", gotOptions.IsoLevel)
	}
	if tx.commits != 1 {
		t.Fatalf("commits = %d, want 1", tx.commits)
	}
	if tx.rollbacks != 1 {
		t.Fatalf("deferred rollbacks = %d, want 1", tx.rollbacks)
	}
}

func TestScopedRunnerRollsBackCallbackError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("admission failed")
	tx := &fakeTransaction{queryRow: func(_ string, _ ...any) pgx.Row {
		return scanRow(func(dest ...any) error {
			*(dest[0].(*string)) = "account_01"
			*(dest[1].(*string)) = "namespace_01"
			return nil
		})
	}}
	runner := &ScopedRunner{begin: func(context.Context, pgx.TxOptions) (transaction, error) {
		return tx, nil
	}}

	err := runner.WithScope(context.Background(), Scope{AccountID: "account_01", NamespaceID: "namespace_01"}, func(context.Context, DBTX) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WithScope() error = %v, want callback error", err)
	}
	if tx.commits != 0 || tx.rollbacks != 1 {
		t.Fatalf("commits/rollbacks = %d/%d, want 0/1", tx.commits, tx.rollbacks)
	}
}

func TestScopedRunnerRejectsInvalidScopeBeforeBegin(t *testing.T) {
	t.Parallel()

	begins := 0
	runner := &ScopedRunner{begin: func(context.Context, pgx.TxOptions) (transaction, error) {
		begins++
		return &fakeTransaction{}, nil
	}}
	err := runner.WithScope(context.Background(), Scope{AccountID: "account_01"}, func(context.Context, DBTX) error { return nil })
	if !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("WithScope() error = %v, want ErrInvalidScope", err)
	}
	if begins != 0 {
		t.Fatalf("begin called %d times for invalid scope", begins)
	}
}
