package control

import (
	"bytes"
	"fmt"
	"strings"
)

// CredentialSource is the source for a connector credential.
type CredentialSource string

const (
	CredentialSourceEnv         CredentialSource = "env"
	CredentialSourceVaultRef    CredentialSource = "vault_ref"
	CredentialSourcePassthrough CredentialSource = "passthrough"
	CredentialSourceEncrypted   CredentialSource = "encrypted"
	CredentialSourceOAuth       CredentialSource = "oauth"
	CredentialSourceSTS         CredentialSource = "sts"

	// CredentialSourceVault is the legacy spelling persisted by early V1
	// releases. New writes use CredentialSourceVaultRef, but reads and runtime
	// resolution continue to accept both spellings.
	CredentialSourceVault CredentialSource = "vault"
)

// EncryptedBlob stores encrypted credential material for local dev only.
type EncryptedBlob struct {
	Ciphertext []byte
	Nonce      []byte
	KeyID      string
}

// Credential source type constants — which tier of the four-tier model.
const (
	CredSourceEnv         = "env"         // Process environment variable name in SecretRef.
	CredSourceEncrypted   = "encrypted"   // Tier 2: encrypted-at-rest in Kave
	CredSourceVaultRef    = "vault_ref"   // Tier 1: external secret manager (Vault, AWS Secrets, etc.)
	CredSourceOAuth       = "oauth"       // Tier 3: ephemeral via OAuth refresh token
	CredSourceSTS         = "sts"         // Tier 3: ephemeral via cloud STS/AssumeRole
	CredSourcePassthrough = "passthrough" // Tier 4: no stored secret; caller supplies per-request
)

// CanonicalCredentialSource normalizes the V1 source vocabulary. In
// particular, legacy rows containing "vault" are exposed as "vault_ref" so
// callers do not need to understand two spellings for the same source.
func CanonicalCredentialSource(source string) CredentialSource {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case string(CredentialSourceVault), string(CredentialSourceVaultRef):
		return CredentialSourceVaultRef
	case string(CredentialSourceEnv):
		return CredentialSourceEnv
	case string(CredentialSourceEncrypted):
		return CredentialSourceEncrypted
	case string(CredentialSourcePassthrough):
		return CredentialSourcePassthrough
	case string(CredentialSourceOAuth):
		return CredentialSourceOAuth
	case string(CredentialSourceSTS):
		return CredentialSourceSTS
	default:
		return CredentialSource(strings.ToLower(strings.TrimSpace(source)))
	}
}

// Credential status constants.
const (
	CredStatusActive          = "active"
	CredStatusRevoked         = "revoked"
	CredStatusExpired         = "expired"
	CredStatusPendingRotation = "pending_rotation"
)

// ConnectorCredential holds a reference to or encrypted copy of an outbound secret.
// Kave uses this to call external systems on behalf of an agent.
// Supports all four tiers: external reference, encrypted local, ephemeral, pass-through.
type ConnectorCredential struct {
	ID        string
	ProjectID string
	EnvID     string
	Name      string
	Connector string

	// What this credential covers
	ConnectorType string // "openai" | "github" | "postgres" | "stripe" | ...
	AccountID     string // org/account/project within the connector; "" = default
	Label         string // "primary" | "billing" | "canary" | custom
	Description   string

	// New credential model fields.
	Source    CredentialSource
	EnvVar    string
	VaultRef  string
	Encrypted *EncryptedBlob
	Metadata  map[string]any

	// Storage tier discriminator (CredSource* constants)
	SourceType string

	// Tier 2: encrypted local secret
	EncryptedBlob []byte // AES-256-GCM ciphertext; nil for non-encrypted tiers
	KeyHash       string // SHA256(plaintext); nil for non-encrypted tiers; used for dedup
	WrappingKeyID string // which master key encrypted this blob

	// Tier 1 + 3: external reference (never a secret itself)
	SecretRef     string // Vault path, ARN, K8s secret ref, etc.
	SecretVersion string // version at the source; may be empty

	// Lifecycle
	Status          string // CredStatus* constants
	Version         int    // incremented on rotation
	ExpiresAt       *int64 // UnixMilli; nil = no expiry
	RotatedAt       *int64 // UnixMilli; nil = never rotated
	RotatedBy       string // user ID or "system"
	LastUsedAt      *int64 // UnixMilli
	LastValidatedAt *int64 // UnixMilli

	// Provenance
	CreatedBy string
	CreatedAt int64 // UnixMilli
	UpdatedAt int64 // UnixMilli

	// Revocation
	RevokedAt    *int64 // UnixMilli; nil = active
	RevokedBy    string
	RevokeReason string
}

// NormalizeSourceFields reconciles the legacy structured source fields with
// the flat fields used by the V1 stores. It never resolves or returns a secret.
// Unknown/future source values are retained so the runtime can reject them with
// a precise error instead of accidentally treating them as Vault references.
func (c *ConnectorCredential) NormalizeSourceFields() error {
	if c == nil {
		return fmt.Errorf("credential is nil")
	}

	fromSource := CanonicalCredentialSource(string(c.Source))
	fromType := CanonicalCredentialSource(c.SourceType)
	if fromSource != "" && fromType != "" && fromSource != fromType {
		return fmt.Errorf("credential %q has conflicting sources %q and %q", c.ID, c.Source, c.SourceType)
	}

	source := fromType
	if source == "" {
		source = fromSource
	}
	if source == "" {
		return nil
	}
	c.Source = source
	c.SourceType = string(source)

	switch source {
	case CredentialSourceEnv:
		ref, err := consistentCredentialReference(c.ID, "environment variable", c.EnvVar, c.SecretRef)
		if err != nil {
			return err
		}
		c.EnvVar = ref
		c.SecretRef = ref
	case CredentialSourceVaultRef:
		ref, err := consistentCredentialReference(c.ID, "vault reference", c.VaultRef, c.SecretRef)
		if err != nil {
			return err
		}
		c.VaultRef = ref
		c.SecretRef = ref
	case CredentialSourceEncrypted:
		if c.Encrypted != nil && len(c.Encrypted.Ciphertext) > 0 {
			if len(c.EncryptedBlob) > 0 && !bytes.Equal(c.EncryptedBlob, c.Encrypted.Ciphertext) {
				return fmt.Errorf("credential %q has conflicting encrypted blobs", c.ID)
			}
			c.EncryptedBlob = c.Encrypted.Ciphertext
			if c.WrappingKeyID == "" {
				c.WrappingKeyID = c.Encrypted.KeyID
			}
		}
		if len(c.EncryptedBlob) > 0 {
			c.Encrypted = &EncryptedBlob{
				Ciphertext: c.EncryptedBlob,
				KeyID:      c.WrappingKeyID,
			}
		}
	}
	return nil
}

func consistentCredentialReference(id, kind, structured, flat string) (string, error) {
	if structured != "" && flat != "" && structured != flat {
		return "", fmt.Errorf("credential %q has conflicting %s references", id, kind)
	}
	if structured != "" {
		return structured, nil
	}
	return flat, nil
}

// CredentialFilter is used by ResolveCredential to find the best matching active credential.
type CredentialFilter struct {
	EnvID         string
	ConnectorType string
	AccountID     string // "" = match any account
	Label         string // "" = prefer "primary", then first active
	Status        string // default: "active"
}

// Credential is the preferred name in newer auth and config code.
type Credential = ConnectorCredential
