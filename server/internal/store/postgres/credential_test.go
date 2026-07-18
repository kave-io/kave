package postgres

import (
	"testing"

	controlmodel "github.com/kave-io/kave/core/model/control"
)

func TestHydratePostgresCredentialSource(t *testing.T) {
	tests := []struct {
		name      string
		cred      *controlmodel.ConnectorCredential
		want      controlmodel.CredentialSource
		wantEnv   string
		wantVault string
	}{
		{name: "env", cred: &controlmodel.ConnectorCredential{ID: "env", SourceType: "env", SecretRef: "PROVIDER_KEY"}, want: controlmodel.CredentialSourceEnv, wantEnv: "PROVIDER_KEY"},
		{name: "legacy vault", cred: &controlmodel.ConnectorCredential{ID: "vault", SourceType: "vault", SecretRef: "kv/provider"}, want: controlmodel.CredentialSourceVaultRef, wantVault: "kv/provider"},
		{name: "vault ref", cred: &controlmodel.ConnectorCredential{ID: "vault-ref", SourceType: "vault_ref", SecretRef: "kv/provider"}, want: controlmodel.CredentialSourceVaultRef, wantVault: "kv/provider"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := hydratePostgresCredentialSource(test.cred); err != nil {
				t.Fatalf("hydratePostgresCredentialSource() error = %v", err)
			}
			if test.cred.Source != test.want || test.cred.SourceType != string(test.want) {
				t.Fatalf("source = (%q, %q), want %q", test.cred.Source, test.cred.SourceType, test.want)
			}
			if test.cred.EnvVar != test.wantEnv || test.cred.VaultRef != test.wantVault {
				t.Fatalf("references = (env=%q, vault=%q), want (env=%q, vault=%q)", test.cred.EnvVar, test.cred.VaultRef, test.wantEnv, test.wantVault)
			}
		})
	}
}

func TestHydratePostgresCredentialSourceReconstructsEncryptedView(t *testing.T) {
	credential := &controlmodel.ConnectorCredential{
		ID:            "encrypted",
		SourceType:    controlmodel.CredSourceEncrypted,
		EncryptedBlob: []byte("ciphertext"),
		WrappingKeyID: "key-1",
	}
	if err := hydratePostgresCredentialSource(credential); err != nil {
		t.Fatalf("hydratePostgresCredentialSource() error = %v", err)
	}
	if credential.Encrypted == nil || string(credential.Encrypted.Ciphertext) != "ciphertext" || credential.Encrypted.KeyID != "key-1" {
		t.Fatalf("encrypted hydration = %#v", credential.Encrypted)
	}
}
