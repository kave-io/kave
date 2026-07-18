package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	controlmodel "github.com/kave-io/kave/core/model/control"
)

func TestPostgresCredentialSourceStorageAndLegacyHydration(t *testing.T) {
	dsn := os.Getenv("KAVE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KAVE_TEST_POSTGRES_DSN not set; skipping postgres integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	t.Cleanup(pool.Close)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	orgID := "cred_org_" + suffix
	projectID := "cred_project_" + suffix
	envID := "cred_env_" + suffix
	credentialID := "cred_" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM workspaces WHERE id IN ($1, $2)`, projectID, orgID)
	})

	now := time.Now().UnixMilli()
	store := New(pool, dsn)
	if err := store.CreateOrg(ctx, &controlmodel.Organization{ID: orgID, Name: orgID, Slug: orgID, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateOrg() error = %v", err)
	}
	if err := store.CreateProject(ctx, &controlmodel.Project{ID: projectID, OrgID: orgID, Name: projectID, Slug: projectID, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := store.CreateEnvironment(ctx, &controlmodel.Environment{ID: envID, ProjectID: projectID, Name: envID, Slug: envID, Type: controlmodel.EnvTypeDev, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}

	credential := &controlmodel.ConnectorCredential{
		ID:            credentialID,
		ProjectID:     projectID,
		EnvID:         envID,
		ConnectorType: "openai",
		Label:         "primary",
		Source:        controlmodel.CredentialSourceEnv,
		EnvVar:        "OPENAI_API_KEY",
		Status:        controlmodel.CredStatusActive,
		Version:       1,
		CreatedBy:     "test",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.StoreCredential(ctx, credential); err != nil {
		t.Fatalf("StoreCredential(env) error = %v", err)
	}

	var sourceType, secretRef string
	if err := pool.QueryRow(ctx, `SELECT source_type, secret_ref FROM credentials WHERE id = $1`, credentialID).Scan(&sourceType, &secretRef); err != nil {
		t.Fatalf("query credential source: %v", err)
	}
	if sourceType != controlmodel.CredSourceEnv || secretRef != "OPENAI_API_KEY" {
		t.Fatalf("persisted source = (%q, %q), want (env, OPENAI_API_KEY)", sourceType, secretRef)
	}
	hydrated, err := store.GetCredential(ctx, credentialID)
	if err != nil {
		t.Fatalf("GetCredential(env) error = %v", err)
	}
	if hydrated.Source != controlmodel.CredentialSourceEnv || hydrated.EnvVar != "OPENAI_API_KEY" {
		t.Fatalf("hydrated env credential = %#v", hydrated)
	}

	if _, err := pool.Exec(ctx, `UPDATE credentials SET source_type = 'vault', secret_ref = 'kv/provider' WHERE id = $1`, credentialID); err != nil {
		t.Fatalf("install legacy vault source: %v", err)
	}
	legacy, err := store.GetCredential(ctx, credentialID)
	if err != nil {
		t.Fatalf("GetCredential(legacy vault) error = %v", err)
	}
	if legacy.SourceType != controlmodel.CredSourceVaultRef || legacy.Source != controlmodel.CredentialSourceVaultRef || legacy.VaultRef != "kv/provider" {
		t.Fatalf("hydrated legacy vault credential = %#v", legacy)
	}
}
