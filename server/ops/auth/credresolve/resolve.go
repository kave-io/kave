package credresolve

import (
	"context"
	"errors"
	"fmt"
	"os"

	controlmodel "github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/pkg/keyring"
	infraCrypto "github.com/kave-io/kave/server/internal/infra/crypto"
)

var ErrPassthrough = errors.New("passthrough credential")
var ErrSourceDisabled = errors.New("credential source disabled")
var ErrUnsupportedSource = errors.New("unsupported credential source")

// VaultClient resolves external secret references.
type VaultClient interface {
	Resolve(ctx context.Context, ref string) (string, error)
}

// NoopVault returns Unimplemented for every lookup.
type NoopVault struct{}

func (NoopVault) Resolve(context.Context, string) (string, error) {
	return "", ErrSourceDisabled
}

// Resolve returns the raw secret for a credential source.
func Resolve(ctx context.Context, cred *controlmodel.ConnectorCredential, vault VaultClient) (string, error) {
	if cred == nil {
		return "", fmt.Errorf("credential is nil")
	}
	normalized := *cred
	if err := normalized.NormalizeSourceFields(); err != nil {
		return "", fmt.Errorf("normalize credential source: %w", err)
	}
	source := normalized.Source
	if source == "" {
		return "", fmt.Errorf("credential %q has no source", cred.ID)
	}
	switch source {
	case controlmodel.CredentialSourceEnv:
		if normalized.EnvVar == "" {
			return "", fmt.Errorf("credential %q: env var name missing", cred.ID)
		}
		val := os.Getenv(normalized.EnvVar)
		if val == "" {
			return "", fmt.Errorf("credential %q: env var %q is unset", cred.ID, normalized.EnvVar)
		}
		return val, nil
	case controlmodel.CredentialSourceVaultRef:
		if normalized.VaultRef == "" {
			return "", fmt.Errorf("credential %q: vault reference missing", cred.ID)
		}
		if vault == nil {
			return "", ErrSourceDisabled
		}
		return vault.Resolve(ctx, normalized.VaultRef)
	case controlmodel.CredentialSourcePassthrough:
		return "", ErrPassthrough
	case controlmodel.CredentialSourceEncrypted:
		key, err := keyring.GetOrCreateMasterKey(ctx)
		if err != nil {
			return "", err
		}
		if normalized.Encrypted == nil || len(normalized.Encrypted.Ciphertext) == 0 {
			return "", fmt.Errorf("credential %q: encrypted credential missing blob", cred.ID)
		}
		aes, err := infraCrypto.NewAES(key)
		if err != nil {
			return "", err
		}
		plain, err := aes.Decrypt(normalized.Encrypted.Ciphertext, []byte(cred.ID))
		if err != nil {
			return "", err
		}
		return string(plain), nil
	default:
		return "", fmt.Errorf("%w %q for credential %q", ErrUnsupportedSource, source, cred.ID)
	}
}
