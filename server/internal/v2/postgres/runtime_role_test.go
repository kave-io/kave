package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/pkg/ids"
)

func TestRuntimeRoleNameIsStrict(t *testing.T) {
	t.Parallel()
	for _, valid := range []string{"kave_runtime", "runtime2", "_kave"} {
		if !runtimeRolePattern.MatchString(valid) {
			t.Fatalf("valid role %q rejected", valid)
		}
	}
	for _, invalid := range []string{"", "Kave", "kave-runtime", "runtime;DROP ROLE owner", "2runtime"} {
		if runtimeRolePattern.MatchString(invalid) {
			t.Fatalf("invalid role %q accepted", invalid)
		}
	}
}

func TestRuntimeRolePostgres_ExactAndFutureClosedPrivileges(t *testing.T) {
	adminDSN := os.Getenv("KAVE_TEST_V2_POSTGRES_ADMIN_DSN")
	if adminDSN == "" {
		t.Skip("KAVE_TEST_V2_POSTGRES_ADMIN_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	adminPool, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(adminPool.Close)

	suffix := strings.ToLower(ids.New("")[:12])
	databaseName := "kave_v2_role_" + suffix
	ownerRole := "kave_v2_owner_" + suffix
	runtimeRole := "kave_v2_runtime_" + suffix
	memberRole := "kave_v2_member_" + suffix
	owner := pgx.Identifier{ownerRole}.Sanitize()
	runtime := pgx.Identifier{runtimeRole}.Sanitize()
	member := pgx.Identifier{memberRole}.Sanitize()
	database := pgx.Identifier{databaseName}.Sanitize()

	var migrationLogin string
	if err := adminPool.QueryRow(ctx, "SELECT current_user").Scan(&migrationLogin); err != nil {
		t.Fatal(err)
	}
	migrationLoginIdentifier := pgx.Identifier{migrationLogin}.Sanitize()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		_, _ = adminPool.Exec(cleanupCtx, "DROP DATABASE IF EXISTS "+database+" WITH (FORCE)")
		_, _ = adminPool.Exec(cleanupCtx, "DROP ROLE IF EXISTS "+runtime)
		_, _ = adminPool.Exec(cleanupCtx, "DROP ROLE IF EXISTS "+member)
		_, _ = adminPool.Exec(cleanupCtx, "DROP ROLE IF EXISTS "+owner)
	})
	for _, statement := range []string{
		"CREATE ROLE " + owner + " NOLOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS",
		"CREATE ROLE " + runtime + " LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD 'kave-v2-runtime-test'",
		"CREATE ROLE " + member + " NOLOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS",
		"GRANT " + owner + " TO " + migrationLoginIdentifier,
		"CREATE DATABASE " + database,
		"GRANT CREATE ON DATABASE " + database + " TO " + owner,
	} {
		if _, err := adminPool.Exec(ctx, statement); err != nil {
			t.Fatalf("test role setup %q: %v", statement, err)
		}
	}

	ownerConfig, err := pgxpool.ParseConfig(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	ownerConfig.ConnConfig.Database = databaseName
	ownerConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET ROLE "+owner)
		return err
	}
	ownerPool, err := pgxpool.NewWithConfig(ctx, ownerConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer ownerPool.Close()

	migrator, err := NewMigrator(ownerPool)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := migrator.GrantRuntime(ctx, runtimeRole); err != nil {
		t.Fatalf("initial runtime grant: %v", err)
	}

	runtimeConfig, err := pgxpool.ParseConfig(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	runtimeConfig.ConnConfig.Database = databaseName
	runtimeConfig.ConnConfig.User = runtimeRole
	runtimeConfig.ConnConfig.Password = "kave-v2-runtime-test"
	runtimePool, err := pgxpool.NewWithConfig(ctx, runtimeConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer runtimePool.Close()
	if err := runtimePool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRuntimeRole(ctx, runtimePool, runtimeRole); err != nil {
		t.Fatalf("baseline runtime verification: %v", err)
	}

	t.Run("role attributes and memberships are rejected", func(t *testing.T) {
		if _, err := adminPool.Exec(ctx, "ALTER ROLE "+runtime+" INHERIT CREATEDB"); err != nil {
			t.Fatal(err)
		}
		if err := VerifyRuntimeRole(ctx, runtimePool, runtimeRole); err == nil {
			t.Fatal("VerifyRuntimeRole accepted INHERIT/CREATEDB")
		}
		if err := migrator.GrantRuntime(ctx, runtimeRole); err == nil {
			t.Fatal("GrantRuntime accepted INHERIT/CREATEDB")
		}
		if _, err := adminPool.Exec(ctx, "ALTER ROLE "+runtime+" NOINHERIT NOCREATEDB"); err != nil {
			t.Fatal(err)
		}
		if _, err := adminPool.Exec(ctx, "GRANT "+member+" TO "+runtime); err != nil {
			t.Fatal(err)
		}
		if err := VerifyRuntimeRole(ctx, runtimePool, runtimeRole); err == nil {
			t.Fatal("VerifyRuntimeRole accepted an auxiliary role membership")
		}
		if err := migrator.GrantRuntime(ctx, runtimeRole); err == nil {
			t.Fatal("GrantRuntime accepted an auxiliary role membership")
		}
		if _, err := adminPool.Exec(ctx, "REVOKE "+member+" FROM "+runtime); err != nil {
			t.Fatal(err)
		}
		if err := migrator.GrantRuntime(ctx, runtimeRole); err != nil {
			t.Fatalf("restore exact runtime grant: %v", err)
		}
		if err := VerifyRuntimeRole(ctx, runtimePool, runtimeRole); err != nil {
			t.Fatalf("verification after role restoration: %v", err)
		}
	})

	testCases := []struct {
		name       string
		statements []string
	}{
		{
			name: "missing lookup execute",
			statements: []string{
				"REVOKE EXECUTE ON FUNCTION kave_v2.lookup_service_key(TEXT) FROM " + runtime,
			},
		},
		{
			name: "missing required table update",
			statements: []string{
				"REVOKE UPDATE ON kave_v2.agents FROM " + runtime,
			},
		},
		{
			name: "excess destructive table grant",
			statements: []string{
				"GRANT DELETE ON kave_v2.namespaces TO " + runtime,
			},
		},
		{
			name: "excess column grant",
			statements: []string{
				"GRANT SELECT (checksum) ON kave_v2.schema_migrations TO " + runtime,
			},
		},
		{
			name: "future table grant",
			statements: []string{
				"CREATE TABLE kave_v2.future_runtime_table (id BIGINT PRIMARY KEY)",
				"GRANT SELECT ON kave_v2.future_runtime_table TO " + runtime,
			},
		},
		{
			name: "future view grant",
			statements: []string{
				"CREATE VIEW kave_v2.future_runtime_view AS SELECT id FROM kave_v2.namespaces",
				"GRANT SELECT ON kave_v2.future_runtime_view TO " + runtime,
			},
		},
		{
			name: "future sequence grant",
			statements: []string{
				"CREATE SEQUENCE kave_v2.future_runtime_sequence",
				"GRANT USAGE ON SEQUENCE kave_v2.future_runtime_sequence TO " + runtime,
			},
		},
		{
			name: "future function public grant",
			statements: []string{
				"CREATE FUNCTION kave_v2.future_runtime_function() RETURNS INTEGER LANGUAGE SQL AS 'SELECT 1'",
				"GRANT EXECUTE ON FUNCTION kave_v2.future_runtime_function() TO PUBLIC",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			for _, statement := range testCase.statements {
				if _, err := ownerPool.Exec(ctx, statement); err != nil {
					t.Fatalf("inject privilege %q: %v", statement, err)
				}
			}
			if err := VerifyRuntimeRole(ctx, runtimePool, runtimeRole); err == nil {
				t.Fatal("VerifyRuntimeRole accepted a non-exact runtime grant")
			}
			if err := migrator.GrantRuntime(ctx, runtimeRole); err != nil {
				t.Fatalf("converge runtime grant: %v", err)
			}
			if err := VerifyRuntimeRole(ctx, runtimePool, runtimeRole); err != nil {
				t.Fatalf("verification after convergence: %v", err)
			}
		})
	}
}
