package v2

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const maxSecretBytes = 64 << 10

var (
	ErrSecretEncryptionUnavailable  = errors.New("kave v2: encrypted secret storage unavailable")
	ErrSecretIdempotencyUnavailable = errors.New("kave v2: secret idempotency authentication unavailable")
	ErrSecretValidationUnavailable  = errors.New("kave v2: secret validation unavailable")
	ErrSecretNotFound               = errors.New("kave v2: secret not found")
)

type SecretSource string

const (
	SecretEncrypted SecretSource = "encrypted"
	SecretExternal  SecretSource = "external"
)

type SecretMetadata struct {
	ID        string
	Name      Ref
	Source    SecretSource
	Version   int64
	Status    string
	UpdatedAt int64
	Replayed  bool
}

// PutSecretRequest contains write-only secret material. Plaintext is excluded
// from serialization and must never enter logs, audit details, or errors.
type PutSecretRequest struct {
	Caller         Caller
	NamespaceID    Ref
	Name           Ref
	Plaintext      []byte `json:"-"`
	ExternalURI    string
	Validate       bool
	IdempotencyKey Ref
}

func (r PutSecretRequest) ValidateRequest() error {
	if err := r.Caller.AuthorizeControl(r.NamespaceID, OperationSecretsWrite); err != nil {
		return err
	}
	if err := r.Name.ValidateName("secret.name", true); err != nil {
		return err
	}
	if err := r.IdempotencyKey.Validate("idempotency_key", true); err != nil {
		return err
	}
	hasPlaintext := len(r.Plaintext) > 0
	hasExternal := r.ExternalURI != ""
	if hasPlaintext == hasExternal {
		return invalid("secret.value", "must contain exactly one plaintext value or external URI")
	}
	if len(r.Plaintext) > maxSecretBytes {
		return invalid("secret.plaintext", fmt.Sprintf("must be at most %d bytes", maxSecretBytes))
	}
	if hasExternal {
		if err := validateExternalSecretURI(r.ExternalURI); err != nil {
			return err
		}
	}
	return nil
}

func (r PutSecretRequest) Source() SecretSource {
	if r.ExternalURI != "" {
		return SecretExternal
	}
	return SecretEncrypted
}

type RevokeSecretRequest struct {
	Caller Caller
	ID     Ref
	Reason string
}

func (r RevokeSecretRequest) ValidateRequest() error {
	if r.Caller.Bootstrap {
		return fmt.Errorf("%w: bootstrap credential cannot revoke namespace resources", ErrUnauthorized)
	}
	if err := r.Caller.AuthorizeControl(r.Caller.NamespaceID, OperationSecretsWrite); err != nil {
		return err
	}
	if err := r.ID.Validate("secret.id", true); err != nil {
		return err
	}
	if len(r.Reason) > 256 || strings.ContainsAny(r.Reason, "\r\n") {
		return invalid("reason", "must be at most 256 bytes on one line")
	}
	return nil
}

type SecretStore interface {
	PutSecret(context.Context, PutSecretRequest) (SecretMetadata, error)
	RevokeSecret(context.Context, RevokeSecretRequest) error
}

type SecretService struct{ store SecretStore }

func NewSecretService(store SecretStore) *SecretService { return &SecretService{store: store} }

func (s *SecretService) PutSecret(ctx context.Context, req PutSecretRequest) (SecretMetadata, error) {
	if err := req.ValidateRequest(); err != nil {
		return SecretMetadata{}, err
	}
	if s == nil || s.store == nil {
		return SecretMetadata{}, errors.New("kave v2: secret store unavailable")
	}
	return s.store.PutSecret(ctx, req)
}

func (s *SecretService) RevokeSecret(ctx context.Context, req RevokeSecretRequest) error {
	if err := req.ValidateRequest(); err != nil {
		return err
	}
	if s == nil || s.store == nil {
		return errors.New("kave v2: secret store unavailable")
	}
	return s.store.RevokeSecret(ctx, req)
}

// AuthorizeControl enforces the account/namespace binding shared by V2 control
// operations. Bootstrap may select a namespace only within its configured
// account; persisted service keys are bound to exactly one namespace.
func (c Caller) AuthorizeControl(namespaceID Ref, operation Operation) error {
	if err := c.AccountID.Validate("caller.account_id", true); err != nil {
		return err
	}
	if err := namespaceID.Validate("namespace_id", true); err != nil {
		return err
	}
	if err := c.ServiceKeyID.Validate("caller.service_key_id", true); err != nil {
		return err
	}
	if !c.Allows(operation, "") {
		return ErrUnauthorized
	}
	if !c.Bootstrap && c.NamespaceID != namespaceID {
		return fmt.Errorf("%w: service key belongs to another namespace", ErrUnauthorized)
	}
	return nil
}

func validateExternalSecretURI(raw string) error {
	if len(raw) > 2048 || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\r\n") {
		return invalid("secret.external_uri", "is invalid")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.User != nil || (u.Host == "" && u.Opaque == "" && u.Path == "") {
		return invalid("secret.external_uri", "must be an absolute secret-manager URI without userinfo")
	}
	switch strings.ToLower(u.Scheme) {
	case "vault", "aws-secretsmanager", "gcp-secretmanager", "azure-keyvault":
		return nil
	default:
		return invalid("secret.external_uri", "uses an unsupported secret-manager scheme")
	}
}
