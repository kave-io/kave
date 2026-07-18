package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var runtimeRolePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// GrantRuntime grants the deliberately small DML surface after migrations.
// It refuses superusers, BYPASSRLS roles, migration owners, and roles that
// inherit either category. The migration connection should be closed as soon
// as this call succeeds.
func (m *Migrator) GrantRuntime(ctx context.Context, runtimeRole string) error {
	if m == nil || m.begin == nil {
		return ErrNilPool
	}
	if !runtimeRolePattern.MatchString(runtimeRole) {
		return errors.New("v2 postgres: runtime role must be a lowercase PostgreSQL identifier")
	}
	tx, err := m.begin(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("v2 postgres: begin runtime grant: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var migrationRole, migrationSession string
	var migrationCanLogin, migrationSuperuser, migrationBypassRLS bool
	var migrationCreateRole, migrationCreateDB, migrationReplication bool
	if err := tx.QueryRow(ctx, `
SELECT current_user, session_user, owner.rolcanlogin, owner.rolsuper,
       owner.rolbypassrls, owner.rolcreaterole, owner.rolcreatedb,
       owner.rolreplication
FROM pg_catalog.pg_roles AS owner
WHERE owner.rolname = current_user
`).Scan(
		&migrationRole, &migrationSession, &migrationCanLogin, &migrationSuperuser,
		&migrationBypassRLS, &migrationCreateRole, &migrationCreateDB, &migrationReplication,
	); err != nil {
		return fmt.Errorf("v2 postgres: inspect migration owner: %w", err)
	}
	if migrationRole == migrationSession || migrationCanLogin || migrationSuperuser || migrationBypassRLS || migrationCreateRole || migrationCreateDB || migrationReplication {
		return errors.New("v2 postgres: migrations must SET ROLE to a dedicated NOLOGIN, non-privileged schema owner")
	}

	var canLogin, inherit, superuser, bypassRLS, createRole, createDB, replication bool
	err = tx.QueryRow(ctx, `
SELECT role.rolcanlogin, role.rolinherit, role.rolsuper, role.rolbypassrls,
       role.rolcreaterole, role.rolcreatedb, role.rolreplication
FROM pg_catalog.pg_roles AS role
WHERE role.rolname = $1
`, runtimeRole).Scan(&canLogin, &inherit, &superuser, &bypassRLS, &createRole, &createDB, &replication)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("v2 postgres: runtime role %q does not exist", runtimeRole)
	}
	if err != nil {
		return fmt.Errorf("v2 postgres: inspect runtime role: %w", err)
	}
	if runtimeRole == migrationRole {
		return errors.New("v2 postgres: runtime role must differ from migration role")
	}
	if !canLogin || inherit || superuser || bypassRLS || createRole || createDB || replication {
		return errors.New("v2 postgres: runtime role must be LOGIN, NOINHERIT, and have no administrative attributes")
	}

	var hasMembership, inheritsPrivileged, inheritsMigrationOwner bool
	if err := tx.QueryRow(ctx, `
SELECT
	EXISTS (
		SELECT 1
		FROM pg_catalog.pg_auth_members AS membership
		JOIN pg_catalog.pg_roles AS member ON member.oid = membership.member
		WHERE member.rolname = $1
	),
    EXISTS (
        SELECT 1 FROM pg_catalog.pg_roles AS inherited
		WHERE (
			inherited.rolsuper OR inherited.rolbypassrls OR inherited.rolcreaterole
			OR inherited.rolcreatedb OR inherited.rolreplication
			OR inherited.oid IN (
				SELECT namespace.nspowner
				FROM pg_catalog.pg_namespace AS namespace
				WHERE namespace.nspname = 'kave_v2'
				UNION
				SELECT object.relowner
				FROM pg_catalog.pg_class AS object
				JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = object.relnamespace
				WHERE namespace.nspname = 'kave_v2'
				UNION
				SELECT procedure.proowner
				FROM pg_catalog.pg_proc AS procedure
				JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = procedure.pronamespace
				WHERE namespace.nspname = 'kave_v2'
			)
		  )
		  AND inherited.rolname <> $1
          AND pg_catalog.pg_has_role($1, inherited.rolname, 'MEMBER')
	),
	pg_catalog.pg_has_role($1, current_user::TEXT, 'MEMBER')
`, runtimeRole).Scan(&hasMembership, &inheritsPrivileged, &inheritsMigrationOwner); err != nil {
		return fmt.Errorf("v2 postgres: inspect runtime memberships: %w", err)
	}
	if hasMembership || inheritsPrivileged || inheritsMigrationOwner {
		return errors.New("v2 postgres: runtime role must not be a member of another role")
	}

	role := pgx.Identifier{runtimeRole}.Sanitize()
	statements := []string{
		// Make this operation convergent. A previous manual grant must not
		// silently survive and expand the serving process's authority.
		"REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA kave_v2 FROM " + role,
		"REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA kave_v2 FROM " + role,
		"REVOKE ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA kave_v2 FROM " + role,
		"REVOKE ALL PRIVILEGES ON SCHEMA kave_v2 FROM " + role,
		// PUBLIC grants are effective for every role and would survive a
		// role-specific revoke. Remove them now and from future owner-created
		// objects so an added function or relation cannot silently widen the
		// runtime surface between migration and serving startup.
		"REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA kave_v2 FROM PUBLIC",
		"REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA kave_v2 FROM PUBLIC",
		"REVOKE ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA kave_v2 FROM PUBLIC",
		"REVOKE ALL PRIVILEGES ON SCHEMA kave_v2 FROM PUBLIC",
		"ALTER DEFAULT PRIVILEGES IN SCHEMA kave_v2 REVOKE ALL PRIVILEGES ON TABLES FROM PUBLIC",
		"ALTER DEFAULT PRIVILEGES IN SCHEMA kave_v2 REVOKE ALL PRIVILEGES ON SEQUENCES FROM PUBLIC",
		"ALTER DEFAULT PRIVILEGES IN SCHEMA kave_v2 REVOKE ALL PRIVILEGES ON FUNCTIONS FROM PUBLIC",
		"GRANT USAGE ON SCHEMA kave_v2 TO " + role,
		"GRANT SELECT, INSERT, UPDATE ON " +
			"kave_v2.namespaces, kave_v2.secrets, kave_v2.provider_routes, " +
			"kave_v2.agents, kave_v2.service_keys, kave_v2.limits, " +
			"kave_v2.limit_windows, kave_v2.invocations TO " + role,
		"GRANT SELECT, INSERT ON kave_v2.usage_entries, kave_v2.audit_events TO " + role,
		"GRANT EXECUTE ON FUNCTION kave_v2.lookup_service_key(TEXT) TO " + role,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("v2 postgres: grant runtime privileges: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("v2 postgres: commit runtime grants: %w", err)
	}
	return nil
}

// VerifyRuntimeRole fails startup if the serving pool is privileged, owns any
// V2 object, inherits its owner, can create schema objects, or has destructive
// privileges that are outside the exact runtime grant installed above.
func VerifyRuntimeRole(ctx context.Context, pool *pgxpool.Pool, expectedRole string) error {
	if pool == nil {
		return ErrNilPool
	}
	if !runtimeRolePattern.MatchString(expectedRole) {
		return errors.New("v2 postgres: expected runtime role is invalid")
	}
	var currentRole, sessionRole, ownerRole string
	var inherit, superuser, bypassRLS, createRole, createDB, replication bool
	var ownsSchema, inheritsOwner, ownsObject, inheritsObjectOwner, schemaCreate, schemaUsage bool
	var hasMembership, relationPrivilegesMismatch, columnPrivilegesPresent, functionPrivilegesMismatch bool
	err := pool.QueryRow(ctx, `
WITH expected_relation_privileges (object_name, object_kind, privilege_type) AS (
	VALUES
		('namespaces', 'r'::"char", 'SELECT'),
		('namespaces', 'r'::"char", 'INSERT'),
		('namespaces', 'r'::"char", 'UPDATE'),
		('secrets', 'r'::"char", 'SELECT'),
		('secrets', 'r'::"char", 'INSERT'),
		('secrets', 'r'::"char", 'UPDATE'),
		('provider_routes', 'r'::"char", 'SELECT'),
		('provider_routes', 'r'::"char", 'INSERT'),
		('provider_routes', 'r'::"char", 'UPDATE'),
		('agents', 'r'::"char", 'SELECT'),
		('agents', 'r'::"char", 'INSERT'),
		('agents', 'r'::"char", 'UPDATE'),
		('service_keys', 'r'::"char", 'SELECT'),
		('service_keys', 'r'::"char", 'INSERT'),
		('service_keys', 'r'::"char", 'UPDATE'),
		('limits', 'r'::"char", 'SELECT'),
		('limits', 'r'::"char", 'INSERT'),
		('limits', 'r'::"char", 'UPDATE'),
		('limit_windows', 'r'::"char", 'SELECT'),
		('limit_windows', 'r'::"char", 'INSERT'),
		('limit_windows', 'r'::"char", 'UPDATE'),
		('invocations', 'r'::"char", 'SELECT'),
		('invocations', 'r'::"char", 'INSERT'),
		('invocations', 'r'::"char", 'UPDATE'),
		('usage_entries', 'r'::"char", 'SELECT'),
		('usage_entries', 'r'::"char", 'INSERT'),
		('audit_events', 'r'::"char", 'SELECT'),
		('audit_events', 'r'::"char", 'INSERT')
),
effective_relation_privileges AS (
	SELECT DISTINCT object.relname AS object_name,
		object.relkind AS object_kind,
		acl.privilege_type
	FROM pg_catalog.pg_roles AS runtime
	JOIN pg_catalog.pg_namespace AS namespace ON namespace.nspname = 'kave_v2'
	JOIN pg_catalog.pg_class AS object ON object.relnamespace = namespace.oid
	CROSS JOIN LATERAL pg_catalog.aclexplode(
		COALESCE(
			object.relacl,
			pg_catalog.acldefault(
				CASE WHEN object.relkind = 'S' THEN 'S'::"char" ELSE 'r'::"char" END,
				object.relowner
			)
		)
	) AS acl
	WHERE runtime.rolname = current_user
	  AND object.relkind IN ('r', 'p', 'v', 'm', 'S', 'f')
	  AND (
		acl.grantee = 0
		OR acl.grantee = runtime.oid
		OR pg_catalog.pg_has_role(runtime.oid, acl.grantee, 'USAGE')
	  )
),
expected_function_privileges AS (
	SELECT pg_catalog.to_regprocedure('kave_v2.lookup_service_key(text)') AS object_id
),
effective_function_privileges AS (
	SELECT DISTINCT procedure.oid AS object_id
	FROM pg_catalog.pg_roles AS runtime
	JOIN pg_catalog.pg_namespace AS namespace ON namespace.nspname = 'kave_v2'
	JOIN pg_catalog.pg_proc AS procedure ON procedure.pronamespace = namespace.oid
	CROSS JOIN LATERAL pg_catalog.aclexplode(
		COALESCE(procedure.proacl, pg_catalog.acldefault('f'::"char", procedure.proowner))
	) AS acl
	WHERE runtime.rolname = current_user
	  AND acl.privilege_type = 'EXECUTE'
	  AND (
		acl.grantee = 0
		OR acl.grantee = runtime.oid
		OR pg_catalog.pg_has_role(runtime.oid, acl.grantee, 'USAGE')
	  )
)
SELECT
    current_user,
	session_user,
    owner.rolname,
    runtime.rolsuper,
    runtime.rolbypassrls,
	runtime.rolinherit,
	runtime.rolcreaterole,
	runtime.rolcreatedb,
	runtime.rolreplication,
    namespace.nspowner = runtime.oid,
	pg_catalog.pg_has_role(current_user::TEXT, owner.rolname, 'MEMBER'),
	EXISTS (
		SELECT 1 FROM pg_catalog.pg_class AS object
		WHERE object.relnamespace = namespace.oid
		  AND object.relowner = runtime.oid
	),
	EXISTS (
		SELECT 1 FROM pg_catalog.pg_roles AS inherited
		WHERE inherited.rolname <> current_user
		  AND pg_catalog.pg_has_role(current_user::TEXT, inherited.rolname, 'MEMBER')
		  AND (
			inherited.rolsuper OR inherited.rolbypassrls OR inherited.rolcreaterole
			OR inherited.rolcreatedb OR inherited.rolreplication
			OR inherited.oid IN (
				SELECT object.relowner FROM pg_catalog.pg_class AS object
				WHERE object.relnamespace = namespace.oid
				UNION
				SELECT procedure.proowner FROM pg_catalog.pg_proc AS procedure
				WHERE procedure.pronamespace = namespace.oid
			)
		  )
	),
	pg_catalog.has_schema_privilege(current_user, namespace.oid, 'CREATE'),
	pg_catalog.has_schema_privilege(current_user, namespace.oid, 'USAGE'),
	EXISTS (
		SELECT 1
		FROM pg_catalog.pg_auth_members AS membership
		WHERE membership.member = runtime.oid
	),
	EXISTS (
		(SELECT * FROM expected_relation_privileges
		 EXCEPT SELECT * FROM effective_relation_privileges)
		UNION ALL
		(SELECT * FROM effective_relation_privileges
		 EXCEPT SELECT * FROM expected_relation_privileges)
	),
	EXISTS (
		SELECT 1
		FROM pg_catalog.pg_attribute AS attribute
		JOIN pg_catalog.pg_class AS object ON object.oid = attribute.attrelid
		CROSS JOIN LATERAL pg_catalog.aclexplode(attribute.attacl) AS acl
		WHERE object.relnamespace = namespace.oid
		  AND attribute.attnum > 0
		  AND NOT attribute.attisdropped
		  AND (
			acl.grantee = 0
			OR acl.grantee = runtime.oid
			OR pg_catalog.pg_has_role(runtime.oid, acl.grantee, 'USAGE')
		  )
	),
	EXISTS (
		SELECT object_id FROM expected_function_privileges WHERE object_id IS NULL
		UNION ALL
		(SELECT object_id FROM expected_function_privileges WHERE object_id IS NOT NULL
		 EXCEPT SELECT object_id FROM effective_function_privileges)
		UNION ALL
		(SELECT object_id FROM effective_function_privileges
		 EXCEPT SELECT object_id FROM expected_function_privileges WHERE object_id IS NOT NULL)
	)
FROM pg_catalog.pg_roles AS runtime
JOIN pg_catalog.pg_namespace AS namespace ON namespace.nspname = 'kave_v2'
JOIN pg_catalog.pg_roles AS owner ON owner.oid = namespace.nspowner
WHERE runtime.rolname = current_user
`).Scan(
		&currentRole, &sessionRole, &ownerRole, &superuser, &bypassRLS, &inherit, &createRole, &createDB, &replication,
		&ownsSchema, &inheritsOwner, &ownsObject, &inheritsObjectOwner, &schemaCreate, &schemaUsage,
		&hasMembership, &relationPrivilegesMismatch, &columnPrivilegesPresent, &functionPrivilegesMismatch,
	)
	if err != nil {
		return fmt.Errorf("v2 postgres: verify runtime role: %w", err)
	}
	if currentRole != expectedRole || sessionRole != expectedRole {
		return fmt.Errorf("v2 postgres: connected as current_user=%q session_user=%q, expected direct runtime login %q", currentRole, sessionRole, expectedRole)
	}
	if inherit || superuser || bypassRLS || createRole || createDB || replication || hasMembership || ownsSchema || inheritsOwner || ownsObject || inheritsObjectOwner || schemaCreate || !schemaUsage || relationPrivilegesMismatch || columnPrivilegesPresent || functionPrivilegesMismatch {
		return fmt.Errorf("v2 postgres: runtime role %q has authority outside the kernel runtime grant (schema owner %q)", currentRole, ownerRole)
	}
	return nil
}
