package control

import "testing"

func TestNormalizeSourceFieldsCanonicalizesVaultSpellings(t *testing.T) {
	for _, source := range []string{"vault", "vault_ref", " VAULT_REF "} {
		t.Run(source, func(t *testing.T) {
			credential := &ConnectorCredential{ID: "provider", SourceType: source, SecretRef: "secret/provider"}
			if err := credential.NormalizeSourceFields(); err != nil {
				t.Fatalf("NormalizeSourceFields() error = %v", err)
			}
			if credential.Source != CredentialSourceVaultRef || credential.SourceType != CredSourceVaultRef {
				t.Fatalf("source = (%q, %q), want canonical vault_ref", credential.Source, credential.SourceType)
			}
			if credential.VaultRef != "secret/provider" {
				t.Fatalf("VaultRef = %q, want secret/provider", credential.VaultRef)
			}
		})
	}
}

func TestNormalizeSourceFieldsHydratesEnvAndEncrypted(t *testing.T) {
	env := &ConnectorCredential{ID: "env", SourceType: CredSourceEnv, SecretRef: "PROVIDER_KEY"}
	if err := env.NormalizeSourceFields(); err != nil {
		t.Fatalf("normalize env: %v", err)
	}
	if env.Source != CredentialSourceEnv || env.EnvVar != "PROVIDER_KEY" {
		t.Fatalf("env hydration = %#v", env)
	}

	encrypted := &ConnectorCredential{ID: "encrypted", SourceType: CredSourceEncrypted, EncryptedBlob: []byte("ciphertext"), WrappingKeyID: "key-1"}
	if err := encrypted.NormalizeSourceFields(); err != nil {
		t.Fatalf("normalize encrypted: %v", err)
	}
	if encrypted.Source != CredentialSourceEncrypted || encrypted.Encrypted == nil {
		t.Fatalf("encrypted hydration = %#v", encrypted)
	}
	if string(encrypted.Encrypted.Ciphertext) != "ciphertext" || encrypted.Encrypted.KeyID != "key-1" {
		t.Fatalf("encrypted fields = %#v", encrypted.Encrypted)
	}
}

func TestNormalizeSourceFieldsRejectsConflictingVocabulary(t *testing.T) {
	credential := &ConnectorCredential{
		ID:         "provider",
		Source:     CredentialSourceEnv,
		SourceType: CredSourceVaultRef,
	}
	if err := credential.NormalizeSourceFields(); err == nil {
		t.Fatal("expected conflicting source error")
	}
}
