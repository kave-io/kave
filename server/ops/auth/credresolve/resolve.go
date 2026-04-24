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
	source := cred.Source
	if source == "" {
		source = controlmodel.CredentialSource(cred.SourceType)
	}
	if source == "" {
		return "", fmt.Errorf("credential %q has no source", cred.ID)
	}
	switch source {
	case controlmodel.CredentialSourceEnv:
		if cred.EnvVar == "" {
			return "", fmt.Errorf("env var name missing")
		}
		val := os.Getenv(cred.EnvVar)
		if val == "" {
			return "", fmt.Errorf("env var unset")
		}
		return val, nil
	case controlmodel.CredentialSourceVault:
		if vault == nil {
			return "", ErrSourceDisabled
		}
		return vault.Resolve(ctx, cred.VaultRef)
	case controlmodel.CredentialSourcePassthrough:
		return "", ErrPassthrough
	case controlmodel.CredentialSourceEncrypted:
		key, err := keyring.GetOrCreateMasterKey(ctx)
		if err != nil {
			return "", err
		}
		if cred.Encrypted == nil || len(cred.Encrypted.Ciphertext) == 0 {
			return "", fmt.Errorf("encrypted credential missing blob")
		}
		aes, err := infraCrypto.NewAES(key)
		if err != nil {
			return "", err
		}
		plain, err := aes.Decrypt(cred.Encrypted.Ciphertext, []byte(cred.ID))
		if err != nil {
			return "", err
		}
		return string(plain), nil
	default:
		return "", fmt.Errorf("unsupported credential source %q", source)
	}
}
