package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	controlmodel "github.com/kave-io/kave/core/model/control"
)

func TestCredentialSourceStorageAndLegacyHydration(t *testing.T) {
	ctx := context.Background()
	store, err := New(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UnixMilli()
	if err := store.CreateOrg(ctx, &controlmodel.Organization{ID: "org", Name: "org", Slug: "org", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateOrg() error = %v", err)
	}
	if err := store.CreateProject(ctx, &controlmodel.Project{ID: "project", OrgID: "org", Name: "project", Slug: "project", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := store.CreateEnvironment(ctx, &controlmodel.Environment{ID: "env", ProjectID: "project", Name: "env", Slug: "env", Type: controlmodel.EnvTypeDev, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}

	credential := &controlmodel.ConnectorCredential{
		ID:            "provider",
		ProjectID:     "project",
		EnvID:         "env",
		ConnectorType: "openai",
		Label:         "primary",
		SourceType:    controlmodel.CredSourceVaultRef,
		SecretRef:     "kv/provider",
		Status:        controlmodel.CredStatusActive,
		Version:       1,
		CreatedBy:     "test",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.StoreCredential(ctx, credential); err != nil {
		t.Fatalf("StoreCredential(vault_ref) error = %v", err)
	}

	var persistedSource string
	if err := store.db.QueryRowContext(ctx, `SELECT source_type FROM credentials WHERE id = ?`, credential.ID).Scan(&persistedSource); err != nil {
		t.Fatalf("query source_type: %v", err)
	}
	if persistedSource != controlmodel.CredSourceVaultRef {
		t.Fatalf("persisted source = %q, want vault_ref", persistedSource)
	}

	// Early V1 releases persisted "vault". Hydration must expose the same
	// canonical runtime shape as a new vault_ref row.
	if _, err := store.db.ExecContext(ctx, `UPDATE credentials SET source_type = 'vault' WHERE id = ?`, credential.ID); err != nil {
		t.Fatalf("install legacy source: %v", err)
	}
	legacy, err := store.GetCredential(ctx, credential.ID)
	if err != nil {
		t.Fatalf("GetCredential(legacy vault) error = %v", err)
	}
	if legacy.SourceType != controlmodel.CredSourceVaultRef || legacy.Source != controlmodel.CredentialSourceVaultRef {
		t.Fatalf("legacy source = (%q, %q), want canonical vault_ref", legacy.SourceType, legacy.Source)
	}
	if legacy.VaultRef != "kv/provider" {
		t.Fatalf("legacy VaultRef = %q, want kv/provider", legacy.VaultRef)
	}

	// Upsert must update the discriminator and reference together; previously
	// SQLite left the old vault source in place.
	legacy.Source = controlmodel.CredentialSourceEnv
	legacy.SourceType = controlmodel.CredSourceEnv
	legacy.VaultRef = ""
	legacy.EnvVar = "OPENAI_API_KEY"
	legacy.SecretRef = "OPENAI_API_KEY"
	legacy.UpdatedAt++
	if err := store.StoreCredential(ctx, legacy); err != nil {
		t.Fatalf("StoreCredential(env update) error = %v", err)
	}
	updated, err := store.GetCredential(ctx, credential.ID)
	if err != nil {
		t.Fatalf("GetCredential(env update) error = %v", err)
	}
	if updated.Source != controlmodel.CredentialSourceEnv || updated.EnvVar != "OPENAI_API_KEY" || updated.SecretRef != "OPENAI_API_KEY" {
		t.Fatalf("updated env credential = %#v", updated)
	}
}

func TestEncryptedCredentialHydration(t *testing.T) {
	ctx := context.Background()
	store, err := New(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UnixMilli()
	if err := store.CreateOrg(ctx, &controlmodel.Organization{ID: "org", Name: "org", Slug: "org", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateOrg() error = %v", err)
	}
	if err := store.CreateProject(ctx, &controlmodel.Project{ID: "project", OrgID: "org", Name: "project", Slug: "project", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := store.CreateEnvironment(ctx, &controlmodel.Environment{ID: "env", ProjectID: "project", Name: "env", Slug: "env", Type: controlmodel.EnvTypeDev, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}

	credential := &controlmodel.ConnectorCredential{
		ID:            "encrypted",
		ProjectID:     "project",
		EnvID:         "env",
		ConnectorType: "openai",
		Label:         "primary",
		SourceType:    controlmodel.CredSourceEncrypted,
		EncryptedBlob: []byte("ciphertext"),
		WrappingKeyID: "key-1",
		Status:        controlmodel.CredStatusActive,
		Version:       1,
		CreatedBy:     "test",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.StoreCredential(ctx, credential); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}
	got, err := store.GetCredential(ctx, credential.ID)
	if err != nil {
		t.Fatalf("GetCredential() error = %v", err)
	}
	if got.Encrypted == nil || string(got.Encrypted.Ciphertext) != "ciphertext" || got.Encrypted.KeyID != "key-1" {
		t.Fatalf("encrypted hydration = %#v", got.Encrypted)
	}
}
