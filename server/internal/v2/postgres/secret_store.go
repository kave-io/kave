package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/pkg/ids"
	corev2 "github.com/kave-io/kave/core/v2"
)

type SecretStore struct {
	runner *ScopedRunner
	cipher SecretCipher
	mac    SecretIdempotencyMAC
	now    func() time.Time
}

const maxSecretStoreAttempts = 8

func NewSecretStore(pool *pgxpool.Pool, cipher SecretCipher) (*SecretStore, error) {
	runner, err := NewScopedRunner(pool)
	if err != nil {
		return nil, err
	}
	mac, _ := cipher.(SecretIdempotencyMAC)
	return &SecretStore{runner: runner, cipher: cipher, mac: mac, now: time.Now}, nil
}

type secretAuditDetails struct {
	RequestHash string              `json:"request_hash"`
	ID          string              `json:"secret_id"`
	Name        corev2.Ref          `json:"name"`
	Source      corev2.SecretSource `json:"source"`
	Version     int64               `json:"version"`
	Status      string              `json:"status"`
	UpdatedAt   int64               `json:"updated_at_ms"`
}

func (s *SecretStore) PutSecret(ctx context.Context, req corev2.PutSecretRequest) (corev2.SecretMetadata, error) {
	if err := req.ValidateRequest(); err != nil {
		return corev2.SecretMetadata{}, err
	}
	if req.Validate {
		// Provider validation must be implemented by a provider-specific
		// validator outside this database transaction. Never claim success when
		// only ciphertext persistence was checked.
		return corev2.SecretMetadata{}, corev2.ErrSecretValidationUnavailable
	}
	if s == nil || s.runner == nil || s.mac == nil {
		return corev2.SecretMetadata{}, corev2.ErrSecretIdempotencyUnavailable
	}
	if req.Source() == corev2.SecretEncrypted && s.cipher == nil {
		return corev2.SecretMetadata{}, corev2.ErrSecretEncryptionUnavailable
	}
	requestHash, err := secretRequestMAC(s.mac, req)
	if err != nil {
		return corev2.SecretMetadata{}, err
	}

	for attempt := 1; attempt <= maxSecretStoreAttempts; attempt++ {
		var result corev2.SecretMetadata
		err = s.runner.WithScope(ctx, Scope{
			AccountID: string(req.Caller.AccountID), NamespaceID: string(req.NamespaceID),
		}, func(txCtx context.Context, db DBTX) error {
			var locked, namespaceStatus string
			if err := db.QueryRow(txCtx, `
SELECT id, status FROM kave_v2.namespaces
WHERE account_id = $1 AND id = $2
FOR UPDATE
`, req.Caller.AccountID, req.NamespaceID).Scan(&locked, &namespaceStatus); err != nil {
				return fmt.Errorf("v2 postgres: lock secret namespace: %w", err)
			}
			if namespaceStatus != "active" {
				return fmt.Errorf("%w: namespace is not active", corev2.ErrUnauthorized)
			}

			auditID := deterministicAuditID("secret.put", req.Caller.AccountID, req.NamespaceID, req.IdempotencyKey)
			prior, found, err := loadSecretAudit(txCtx, db, req.Caller.AccountID, req.NamespaceID, auditID)
			if err != nil {
				return err
			}
			if found {
				if prior.RequestHash != requestHash {
					return &corev2.IdempotencyConflictError{Key: req.IdempotencyKey}
				}
				result = prior.metadata(true)
				return nil
			}

			now := s.now().UTC()
			secretID, version, err := lockSecretVersion(txCtx, db, req.Caller.AccountID, req.NamespaceID, req.Name)
			if err != nil {
				return err
			}
			if secretID == "" {
				secretID = ids.New("sec")
				version = 1
			} else {
				version++
			}

			var externalRef *string
			var ciphertext []byte
			var wrappingKeyID *string
			if req.Source() == corev2.SecretExternal {
				external := req.ExternalURI
				externalRef = &external
			} else {
				sealed, keyID, err := s.cipher.Seal(txCtx, SecretAAD{
					AccountID: string(req.Caller.AccountID), NamespaceID: string(req.NamespaceID),
					Name: string(req.Name), Version: version,
				}, req.Plaintext)
				if err != nil {
					return fmt.Errorf("v2 postgres: encrypt secret: %w", err)
				}
				ciphertext = sealed
				wrappingKeyID = &keyID
			}

			_, err = db.Exec(txCtx, `
INSERT INTO kave_v2.secrets (
    id, account_id, namespace_id, name, backend, external_ref,
    ciphertext, wrapping_key_id, version, status, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active', $10, $10)
ON CONFLICT (account_id, namespace_id, name) DO UPDATE SET
    backend = EXCLUDED.backend,
    external_ref = EXCLUDED.external_ref,
    ciphertext = EXCLUDED.ciphertext,
    wrapping_key_id = EXCLUDED.wrapping_key_id,
    version = EXCLUDED.version,
    status = 'active',
    last_validated_at = NULL,
    revoked_at = NULL,
    updated_at = EXCLUDED.updated_at
`, secretID, req.Caller.AccountID, req.NamespaceID, req.Name, req.Source(), externalRef,
				ciphertext, wrappingKeyID, version, now)
			if err != nil {
				return fmt.Errorf("v2 postgres: persist secret: %w", err)
			}

			details := secretAuditDetails{
				RequestHash: requestHash, ID: secretID, Name: req.Name, Source: req.Source(),
				Version: version, Status: "active", UpdatedAt: now.UnixMilli(),
			}
			if err := insertSecretAudit(txCtx, db, auditID, req.Caller, req.NamespaceID, req.IdempotencyKey, details); err != nil {
				return err
			}
			result = details.metadata(false)
			return nil
		})
		if err == nil {
			return result, nil
		}
		if !isRetryableTransactionError(err) || attempt == maxSecretStoreAttempts {
			return corev2.SecretMetadata{}, err
		}
	}
	return corev2.SecretMetadata{}, err
}

func (s *SecretStore) RevokeSecret(ctx context.Context, req corev2.RevokeSecretRequest) error {
	if err := req.ValidateRequest(); err != nil {
		return err
	}
	if s == nil || s.runner == nil {
		return ErrNilPool
	}
	var err error
	for attempt := 1; attempt <= maxSecretStoreAttempts; attempt++ {
		err = s.runner.WithScope(ctx, Scope{
			AccountID: string(req.Caller.AccountID), NamespaceID: string(req.Caller.NamespaceID),
		}, func(txCtx context.Context, db DBTX) error {
			var locked, namespaceStatus string
			if err := db.QueryRow(txCtx, `
SELECT id, status FROM kave_v2.namespaces
WHERE account_id = $1 AND id = $2
FOR UPDATE
`, req.Caller.AccountID, req.Caller.NamespaceID).Scan(&locked, &namespaceStatus); err != nil {
				return fmt.Errorf("v2 postgres: lock secret namespace: %w", err)
			}
			if namespaceStatus != "active" {
				return fmt.Errorf("%w: namespace is not active", corev2.ErrUnauthorized)
			}
			now := s.now().UTC()
			tag, err := db.Exec(txCtx, `
UPDATE kave_v2.secrets
SET status = 'revoked', revoked_at = $4, updated_at = $4
WHERE account_id = $1 AND namespace_id = $2 AND id = $3 AND status <> 'revoked'
`, req.Caller.AccountID, req.Caller.NamespaceID, req.ID, now)
			if err != nil {
				return fmt.Errorf("v2 postgres: revoke secret: %w", err)
			}
			if tag.RowsAffected() == 0 {
				var status string
				err := db.QueryRow(txCtx, `
SELECT status FROM kave_v2.secrets
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, req.Caller.AccountID, req.Caller.NamespaceID, req.ID).Scan(&status)
				if errors.Is(err, pgx.ErrNoRows) {
					return corev2.ErrSecretNotFound
				}
				if err != nil {
					return fmt.Errorf("v2 postgres: inspect secret revoke: %w", err)
				}
				return nil
			}
			details, _ := json.Marshal(map[string]any{"reason": req.Reason})
			serviceKeyID := string(req.Caller.ServiceKeyID)
			_, err = db.Exec(txCtx, `
INSERT INTO kave_v2.audit_events (
    id, account_id, namespace_id, service_key_id, event, resource_type,
    resource_id, outcome, details, created_at
) VALUES ($1, $2, $3, $4, 'secret.revoke', 'secret', $5, 'succeeded', $6, $7)
`, ids.New("aud"), req.Caller.AccountID, req.Caller.NamespaceID, serviceKeyID, req.ID, details, now)
			if err != nil {
				return fmt.Errorf("v2 postgres: audit secret revoke: %w", err)
			}
			return nil
		})
		if err == nil {
			return nil
		}
		if !isRetryableTransactionError(err) || attempt == maxSecretStoreAttempts {
			return err
		}
	}
	return err
}

func lockSecretVersion(ctx context.Context, db DBTX, accountID, namespaceID, name corev2.Ref) (string, int64, error) {
	var id string
	var version int64
	err := db.QueryRow(ctx, `
SELECT id, version FROM kave_v2.secrets
WHERE account_id = $1 AND namespace_id = $2 AND name = $3
FOR UPDATE
`, accountID, namespaceID, name).Scan(&id, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, fmt.Errorf("v2 postgres: lock secret: %w", err)
	}
	return id, version, nil
}

func loadSecretAudit(ctx context.Context, db DBTX, accountID, namespaceID corev2.Ref, auditID string) (secretAuditDetails, bool, error) {
	var raw []byte
	err := db.QueryRow(ctx, `
SELECT details FROM kave_v2.audit_events
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, accountID, namespaceID, auditID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return secretAuditDetails{}, false, nil
	}
	if err != nil {
		return secretAuditDetails{}, false, fmt.Errorf("v2 postgres: load secret idempotency: %w", err)
	}
	var details secretAuditDetails
	if err := json.Unmarshal(raw, &details); err != nil || details.ID == "" || details.RequestHash == "" {
		return secretAuditDetails{}, false, errors.New("v2 postgres: corrupt secret idempotency record")
	}
	return details, true, nil
}

func insertSecretAudit(ctx context.Context, db DBTX, auditID string, caller corev2.Caller, namespaceID, requestID corev2.Ref, details secretAuditDetails) error {
	raw, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("v2 postgres: encode secret audit: %w", err)
	}
	var serviceKeyID *string
	if !caller.Bootstrap {
		id := string(caller.ServiceKeyID)
		serviceKeyID = &id
	}
	_, err = db.Exec(ctx, `
INSERT INTO kave_v2.audit_events (
    id, account_id, namespace_id, service_key_id, event, resource_type,
    resource_id, outcome, request_id, details
) VALUES ($1, $2, $3, $4, 'secret.put', 'secret', $5, 'succeeded', $6, $7)
`, auditID, caller.AccountID, namespaceID, serviceKeyID, details.ID, requestID, raw)
	if err != nil {
		return fmt.Errorf("v2 postgres: audit secret write: %w", err)
	}
	return nil
}

func deterministicAuditID(operation string, accountID, namespaceID, key corev2.Ref) string {
	digest := sha256.Sum256([]byte(operation + "\x00" + string(accountID) + "\x00" + string(namespaceID) + "\x00" + string(key)))
	return "aud_" + hex.EncodeToString(digest[:16])
}

func secretRequestMAC(mac SecretIdempotencyMAC, req corev2.PutSecretRequest) (string, error) {
	payload, err := json.Marshal(struct {
		NamespaceID corev2.Ref          `json:"namespace_id"`
		Name        corev2.Ref          `json:"name"`
		Source      corev2.SecretSource `json:"source"`
		Plaintext   []byte              `json:"plaintext,omitempty"`
		ExternalURI string              `json:"external_uri,omitempty"`
		Validate    bool                `json:"validate"`
	}{
		NamespaceID: req.NamespaceID, Name: req.Name, Source: req.Source(),
		Plaintext: req.Plaintext, ExternalURI: req.ExternalURI, Validate: req.Validate,
	})
	if err != nil {
		return "", fmt.Errorf("v2 postgres: encode secret idempotency request: %w", err)
	}
	defer clear(payload)
	digest, err := mac.SecretIdempotencyMAC(payload)
	if err != nil {
		return "", fmt.Errorf("v2 postgres: authenticate secret idempotency request: %w", err)
	}
	return digest, nil
}

func (d secretAuditDetails) metadata(replayed bool) corev2.SecretMetadata {
	return corev2.SecretMetadata{
		ID: d.ID, Name: d.Name, Source: d.Source, Version: d.Version,
		Status: d.Status, UpdatedAt: d.UpdatedAt, Replayed: replayed,
	}
}

var _ corev2.SecretStore = (*SecretStore)(nil)
