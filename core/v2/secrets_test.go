package v2

import (
	"errors"
	"testing"
)

func TestPutSecretValidation(t *testing.T) {
	req := PutSecretRequest{
		Caller: Caller{
			AccountID: "account/acme", NamespaceID: "namespace/prod", ServiceKeyID: "key/admin",
			Operations: []Operation{OperationSecretsWrite},
		},
		NamespaceID:    "namespace/prod",
		Name:           "openai",
		Plaintext:      []byte("super-secret-provider-key"),
		IdempotencyKey: "deploy/1",
	}
	if err := req.ValidateRequest(); err != nil {
		t.Fatal(err)
	}
	req.Plaintext = nil
	req.ExternalURI = "https://example.com/key"
	if !errors.Is(req.ValidateRequest(), ErrInvalidArgument) {
		t.Fatal("HTTP external secret URI was accepted")
	}
}

func TestControlAuthorizationBindsPersistedKeyNamespace(t *testing.T) {
	caller := Caller{
		AccountID: "account/acme", NamespaceID: "namespace/prod", ServiceKeyID: "key/admin",
		Operations: []Operation{OperationSecretsWrite},
	}
	if err := caller.AuthorizeControl("namespace/prod", OperationSecretsWrite); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(caller.AuthorizeControl("namespace/other", OperationSecretsWrite), ErrUnauthorized) {
		t.Fatal("cross-namespace control was accepted")
	}
}
