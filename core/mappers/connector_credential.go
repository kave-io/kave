package mappers

import controlmodel "github.com/kave-io/kave/core/model/control"

// ConnectorCredentialUpsert is control-plane input for storing a connector credential.
type ConnectorCredentialUpsert struct {
	ID            string
	ProjectID     string
	EnvID         string
	ConnectorType string
	AccountID     string
	Label         string
	Description   string
	SourceType    string
	EncryptedBlob []byte
	KeyHash       string
	WrappingKeyID string
	SecretRef     string
	SecretVersion string
	ExpiresAt     *int64
	CreatedBy     string
	CreatedAt     *int64
}

// ConnectorCredentialView is the response-safe shape (no encrypted material).
type ConnectorCredentialView struct {
	ID            string
	ProjectID     string
	EnvID         string
	ConnectorType string
	AccountID     string
	Label         string
	Description   string
	SourceType    string
	Status        string
	Version       int
	ExpiresAt     *int64
	RotatedAt     *int64
	RotatedBy     string
	LastUsedAt    *int64
	CreatedBy     string
	CreatedAt     int64
	RevokedAt     *int64
	RevokedBy     string
	RevokeReason  string
}

// ConnectorCredentialUpsertToModel converts create input to controlmodel.ConnectorCredential.
func ConnectorCredentialUpsertToModel(in *ConnectorCredentialUpsert) *controlmodel.ConnectorCredential {
	if in == nil {
		return nil
	}

	createdAt := msSinceEpoch()
	if in.CreatedAt != nil {
		createdAt = *in.CreatedAt
	}

	createdBy := in.CreatedBy
	if createdBy == "" {
		createdBy = "system"
	}

	return &controlmodel.ConnectorCredential{
		ID:            in.ID,
		ProjectID:     in.ProjectID,
		EnvID:         in.EnvID,
		ConnectorType: in.ConnectorType,
		AccountID:     in.AccountID,
		Label:         in.Label,
		Description:   in.Description,
		SourceType:    in.SourceType,
		EncryptedBlob: in.EncryptedBlob,
		KeyHash:       in.KeyHash,
		WrappingKeyID: in.WrappingKeyID,
		SecretRef:     in.SecretRef,
		SecretVersion: in.SecretVersion,
		Status:        controlmodel.CredStatusActive,
		Version:       1,
		ExpiresAt:     in.ExpiresAt,
		CreatedBy:     createdBy,
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	}
}

// ConnectorCredentialToView converts a stored credential to response-safe data (no encrypted material).
func ConnectorCredentialToView(c *controlmodel.ConnectorCredential) *ConnectorCredentialView {
	if c == nil {
		return nil
	}
	return &ConnectorCredentialView{
		ID:            c.ID,
		ProjectID:     c.ProjectID,
		EnvID:         c.EnvID,
		ConnectorType: c.ConnectorType,
		AccountID:     c.AccountID,
		Label:         c.Label,
		Description:   c.Description,
		SourceType:    c.SourceType,
		Status:        c.Status,
		Version:       c.Version,
		ExpiresAt:     c.ExpiresAt,
		RotatedAt:     c.RotatedAt,
		RotatedBy:     c.RotatedBy,
		LastUsedAt:    c.LastUsedAt,
		CreatedBy:     c.CreatedBy,
		CreatedAt:     c.CreatedAt,
		RevokedAt:     c.RevokedAt,
		RevokedBy:     c.RevokedBy,
		RevokeReason:  c.RevokeReason,
	}
}
