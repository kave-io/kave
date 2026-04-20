package control

// CredentialSource is the source for a connector credential.
type CredentialSource string

const (
	CredentialSourceEnv         CredentialSource = "env"
	CredentialSourceVault       CredentialSource = "vault"
	CredentialSourcePassthrough CredentialSource = "passthrough"
	CredentialSourceEncrypted   CredentialSource = "encrypted"
)

// EncryptedBlob stores encrypted credential material for local dev only.
type EncryptedBlob struct {
	Ciphertext []byte
	Nonce      []byte
	KeyID      string
}

// Credential source type constants — which tier of the four-tier model.
const (
	CredSourceEncrypted   = "encrypted"   // Tier 2: encrypted-at-rest in Kave
	CredSourceVaultRef    = "vault_ref"   // Tier 1: external secret manager (Vault, AWS Secrets, etc.)
	CredSourceOAuth       = "oauth"       // Tier 3: ephemeral via OAuth refresh token
	CredSourceSTS         = "sts"         // Tier 3: ephemeral via cloud STS/AssumeRole
	CredSourcePassthrough = "passthrough" // Tier 4: no stored secret; caller supplies per-request
)

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
