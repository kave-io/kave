package control

import (
	"testing"

	controlmodel "github.com/kave-io/kave/core/model/control"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

func TestCredentialToProtoMapsCanonicalAndLegacySources(t *testing.T) {
	tests := []struct {
		name string
		cred *controlmodel.ConnectorCredential
		want controlv1.CredentialSource
	}{
		{name: "env source type", cred: &controlmodel.ConnectorCredential{SourceType: "env"}, want: controlv1.CredentialSource_CREDENTIAL_SOURCE_ENV},
		{name: "env structured", cred: &controlmodel.ConnectorCredential{Source: controlmodel.CredentialSourceEnv}, want: controlv1.CredentialSource_CREDENTIAL_SOURCE_ENV},
		{name: "legacy vault", cred: &controlmodel.ConnectorCredential{SourceType: "vault"}, want: controlv1.CredentialSource_CREDENTIAL_SOURCE_VAULT_REF},
		{name: "canonical vault", cred: &controlmodel.ConnectorCredential{SourceType: "vault_ref"}, want: controlv1.CredentialSource_CREDENTIAL_SOURCE_VAULT_REF},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := credentialToProto(test.cred).SourceType; got != test.want {
				t.Fatalf("SourceType = %v, want %v", got, test.want)
			}
		})
	}
}

func TestCredentialToProtoNeverReturnsEncryptedSecret(t *testing.T) {
	credential := &controlmodel.ConnectorCredential{
		ID:            "provider",
		SourceType:    controlmodel.CredSourceEncrypted,
		EncryptedBlob: []byte("ciphertext-must-not-leave-server"),
		Encrypted:     &controlmodel.EncryptedBlob{Ciphertext: []byte("structured-ciphertext")},
	}

	got := credentialToProto(credential)
	if len(got.EncryptedBlob) != 0 {
		t.Fatalf("EncryptedBlob returned by control API: %q", got.EncryptedBlob)
	}
}

func TestCredentialSourceFromProtoIncludesEnv(t *testing.T) {
	if got := credentialSourceFromProto(controlv1.CredentialSource_CREDENTIAL_SOURCE_ENV); got != controlmodel.CredSourceEnv {
		t.Fatalf("credentialSourceFromProto(env) = %q, want %q", got, controlmodel.CredSourceEnv)
	}
	model := protoToCredential(&controlv1.ConnectorCredential{
		Id:         "provider",
		SourceType: controlv1.CredentialSource_CREDENTIAL_SOURCE_ENV,
		SecretRef:  "PROVIDER_KEY",
	})
	if model.Source != controlmodel.CredentialSourceEnv || model.EnvVar != "PROVIDER_KEY" {
		t.Fatalf("protoToCredential(env) = %#v", model)
	}
}
