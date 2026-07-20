package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kave-io/kave/core/pkg/ids"
	corev2 "github.com/kave-io/kave/core/v2"
)

const maxProviderRouteValidationModels = 256

type routeModelValidationEvidence struct {
	SecretVersion     int64  `json:"secret_version"`
	ValidatedAtMS     int64  `json:"validated_at_ms"`
	HTTPStatus        int    `json:"http_status"`
	ProviderRequestID string `json:"provider_request_id,omitempty"`
}

type routeValidationAttemptEvidence struct {
	Model             string `json:"model"`
	SecretVersion     int64  `json:"secret_version"`
	AttemptedAtMS     int64  `json:"attempted_at_ms"`
	HTTPStatus        int    `json:"http_status,omitempty"`
	ProviderRequestID string `json:"provider_request_id,omitempty"`
	Validated         bool   `json:"validated"`
}

// routeValidationDocument keeps model-specific activation evidence on the
// route without adding another mutable product table. Apply and secret writes
// clear this complete document whenever provider topology, model policy, or
// credential material changes.
type routeValidationDocument struct {
	Models      map[string]routeModelValidationEvidence `json:"models,omitempty"`
	LastAttempt routeValidationAttemptEvidence          `json:"last_attempt"`
}

func decodeRouteValidationDocument(raw []byte) (routeValidationDocument, error) {
	document := routeValidationDocument{Models: make(map[string]routeModelValidationEvidence)}
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("{}")) {
		return document, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return routeValidationDocument{}, fmt.Errorf("decode provider validation evidence: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return routeValidationDocument{}, errors.New("decode provider validation evidence: trailing JSON value")
	}
	if document.Models == nil {
		document.Models = make(map[string]routeModelValidationEvidence)
	}
	if len(document.Models) > maxProviderRouteValidationModels {
		return routeValidationDocument{}, errors.New("provider validation evidence exceeds model bound")
	}
	for model, evidence := range document.Models {
		if err := corev2.Ref(model).Validate("provider_validation.model", true); err != nil ||
			evidence.SecretVersion <= 0 || evidence.ValidatedAtMS <= 0 ||
			evidence.HTTPStatus < 200 || evidence.HTTPStatus >= 300 ||
			sanitizeProviderRequestID(evidence.ProviderRequestID) != evidence.ProviderRequestID {
			return routeValidationDocument{}, errors.New("provider validation evidence is invalid")
		}
	}
	if attempt := document.LastAttempt; attempt.Model != "" {
		if err := corev2.Ref(attempt.Model).Validate("provider_validation.last_attempt.model", true); err != nil ||
			attempt.SecretVersion <= 0 || attempt.AttemptedAtMS <= 0 ||
			(attempt.HTTPStatus != 0 && (attempt.HTTPStatus < 100 || attempt.HTTPStatus > 599)) ||
			sanitizeProviderRequestID(attempt.ProviderRequestID) != attempt.ProviderRequestID {
			return routeValidationDocument{}, errors.New("provider validation attempt evidence is invalid")
		}
	}
	return document, nil
}

func (d routeValidationDocument) validates(model string, secretVersion int64) bool {
	evidence, ok := d.Models[model]
	return ok && secretVersion > 0 && evidence.SecretVersion == secretVersion &&
		evidence.ValidatedAtMS > 0 && evidence.HTTPStatus >= 200 && evidence.HTTPStatus < 300
}

func (d routeValidationDocument) latest() (string, int64, time.Time) {
	var model string
	var evidence routeModelValidationEvidence
	for candidate, item := range d.Models {
		if item.ValidatedAtMS > evidence.ValidatedAtMS ||
			(item.ValidatedAtMS == evidence.ValidatedAtMS && (model == "" || candidate < model)) {
			model, evidence = candidate, item
		}
	}
	if model == "" {
		return "", 0, time.Time{}
	}
	return model, evidence.SecretVersion, time.UnixMilli(evidence.ValidatedAtMS).UTC()
}

// PrepareProviderRouteActivation resolves and decrypts one exact route target
// under RLS. The credential exists only in the returned short-lived grant.
func (s *ProviderStore) PrepareProviderRouteActivation(ctx context.Context, req corev2.ActivateProviderRouteRequest) (corev2.ProviderRouteValidationTarget, error) {
	if err := req.Validate(); err != nil {
		return corev2.ProviderRouteValidationTarget{}, err
	}
	if s == nil || s.runner == nil || s.cipher == nil {
		return corev2.ProviderRouteValidationTarget{}, corev2.ErrSecretEncryptionUnavailable
	}
	var target corev2.ProviderRouteValidationTarget
	err := s.runner.WithScope(ctx, Scope{AccountID: string(req.Caller.AccountID), NamespaceID: string(req.NamespaceID)}, func(txCtx context.Context, db DBTX) error {
		var routeStatus, secretStatus, secretBackend, wrappingKeyID string
		var policyBytes, ciphertext []byte
		err := db.QueryRow(txCtx, `
SELECT r.id, r.revision, r.provider, r.protocol, r.base_url, r.model_policy, r.status,
       s.id, s.name, s.version, s.backend, s.status,
       COALESCE(s.ciphertext, ''::bytea), COALESCE(s.wrapping_key_id, '')
FROM kave_v2.provider_routes r
JOIN kave_v2.secrets s
  ON s.account_id = r.account_id AND s.namespace_id = r.namespace_id AND s.id = r.secret_id
WHERE r.account_id = $1 AND r.namespace_id = $2 AND r.name = $3
FOR UPDATE OF r, s
`, req.Caller.AccountID, req.NamespaceID, req.Route).Scan(
			&target.RouteID, &target.RouteRevision, &target.Provider, &target.Protocol,
			&target.BaseURL, &policyBytes, &routeStatus, &target.SecretID,
			&target.SecretName, &target.SecretVersion, &secretBackend, &secretStatus,
			&ciphertext, &wrappingKeyID,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return corev2.ErrProviderRouteNotFound
		}
		if err != nil {
			return fmt.Errorf("v2 postgres: load provider activation target: %w", err)
		}
		if (routeStatus != "active" && routeStatus != "invalid") || secretStatus != "active" || secretBackend != "encrypted" {
			return corev2.ErrProviderRouteNotFound
		}
		var policy routeModelPolicy
		if err := json.Unmarshal(policyBytes, &policy); err != nil {
			return errors.New("v2 postgres: provider route has invalid model policy")
		}
		model := string(req.Model)
		if model == "" {
			model = policy.DefaultModel
		}
		if !corev2.ModelAllowed(model, policy.AllowedModels) {
			return fmt.Errorf("%w: model is not allowed by route", corev2.ErrInvalidArgument)
		}
		target.AccountID = req.Caller.AccountID
		target.NamespaceID = req.NamespaceID
		target.Route = req.Route
		target.Model = corev2.Ref(model)
		opened, err := s.cipher.Open(txCtx, SecretAAD{
			AccountID: string(req.Caller.AccountID), NamespaceID: string(req.NamespaceID),
			Name: string(target.SecretName), Version: target.SecretVersion,
		}, ciphertext, wrappingKeyID)
		if err != nil {
			clear(opened)
			return corev2.ErrSecretEncryptionUnavailable
		}
		credential, ok := normalizeProviderCredential(opened)
		clear(opened)
		if !ok {
			clear(credential)
			return corev2.ErrProviderValidationFailed
		}
		target.Credential = credential
		return nil
	})
	if err != nil {
		clear(target.Credential)
		return corev2.ProviderRouteValidationTarget{}, err
	}
	if err := target.Validate(); err != nil {
		clear(target.Credential)
		return corev2.ProviderRouteValidationTarget{}, err
	}
	return target, nil
}

// RecordProviderRouteValidation conditionally activates the exact route and
// secret versions that were probed. Concurrent Apply/PutSecret operations make
// the result stale instead of activating unvalidated state.
func (s *ProviderStore) RecordProviderRouteValidation(ctx context.Context, req corev2.ActivateProviderRouteRequest, target corev2.ProviderRouteValidationTarget, evidence corev2.ProviderRouteValidationEvidence, success bool) (corev2.ProviderRouteActivationResult, error) {
	if err := req.Validate(); err != nil {
		return corev2.ProviderRouteActivationResult{}, err
	}
	if err := target.Validate(); err != nil {
		return corev2.ProviderRouteActivationResult{}, err
	}
	if s == nil || s.runner == nil {
		return corev2.ProviderRouteActivationResult{}, ErrNilPool
	}
	evidence = sanitizeProviderValidationEvidence(evidence)
	now := s.now().UTC()
	var result corev2.ProviderRouteActivationResult
	err := s.runner.WithScope(ctx, Scope{AccountID: string(req.Caller.AccountID), NamespaceID: string(req.NamespaceID)}, func(txCtx context.Context, db DBTX) error {
		var routeRevision, secretVersion int64
		var routeName string
		var validationBytes []byte
		err := db.QueryRow(txCtx, `
SELECT r.revision, r.name, s.version, r.validation_evidence
FROM kave_v2.provider_routes r
JOIN kave_v2.secrets s
  ON s.account_id = r.account_id AND s.namespace_id = r.namespace_id AND s.id = r.secret_id
WHERE r.account_id = $1 AND r.namespace_id = $2 AND r.id = $3 AND s.id = $4
FOR UPDATE OF r, s
`, req.Caller.AccountID, req.NamespaceID, target.RouteID, target.SecretID).Scan(&routeRevision, &routeName, &secretVersion, &validationBytes)
		if errors.Is(err, pgx.ErrNoRows) {
			return corev2.ErrProviderActivationStale
		}
		if err != nil {
			return fmt.Errorf("v2 postgres: lock provider activation target: %w", err)
		}
		if routeName != string(req.Route) || routeRevision != target.RouteRevision || secretVersion != target.SecretVersion {
			return corev2.ErrProviderActivationStale
		}
		if success && (evidence.HTTPStatus < 200 || evidence.HTTPStatus >= 300) {
			return corev2.ErrProviderValidationFailed
		}
		document, err := decodeRouteValidationDocument(validationBytes)
		if err != nil {
			return fmt.Errorf("v2 postgres: %w", err)
		}
		// A secret write normally clears the complete document. Filtering here
		// provides a second fail-closed boundary if persisted evidence was ever
		// restored inconsistently.
		for model, prior := range document.Models {
			if prior.SecretVersion != target.SecretVersion {
				delete(document.Models, model)
			}
		}
		document.LastAttempt = routeValidationAttemptEvidence{
			Model: string(target.Model), SecretVersion: target.SecretVersion,
			AttemptedAtMS: now.UnixMilli(), HTTPStatus: evidence.HTTPStatus,
			ProviderRequestID: evidence.ProviderRequestID, Validated: success,
		}
		if success {
			document.Models[string(target.Model)] = routeModelValidationEvidence{
				SecretVersion: target.SecretVersion, ValidatedAtMS: now.UnixMilli(),
				HTTPStatus: evidence.HTTPStatus, ProviderRequestID: evidence.ProviderRequestID,
			}
		} else {
			delete(document.Models, string(target.Model))
		}
		validatedModel, validatedSecretVersion, lastValidatedAt := document.latest()
		status := "invalid"
		outcome := "failed"
		if len(document.Models) > 0 {
			status = "active"
		}
		if success {
			outcome = "succeeded"
		}
		auditDetail := map[string]any{
			"provider": target.Provider, "model": target.Model,
			"route_revision": target.RouteRevision, "secret_version": target.SecretVersion,
			"http_status": evidence.HTTPStatus, "provider_request_id": evidence.ProviderRequestID,
			"validated": success,
		}
		auditDetailBytes, err := json.Marshal(auditDetail)
		if err != nil {
			return fmt.Errorf("v2 postgres: encode provider validation evidence: %w", err)
		}
		validationBytes, err = json.Marshal(document)
		if err != nil {
			return fmt.Errorf("v2 postgres: encode model validation document: %w", err)
		}
		var validatedAtValue any
		var validatedVersionValue any
		var validatedModelValue any
		if !lastValidatedAt.IsZero() {
			validatedAtValue = lastValidatedAt
			validatedVersionValue = validatedSecretVersion
			validatedModelValue = validatedModel
		}
		var nextRevision int64
		err = db.QueryRow(txCtx, `
UPDATE kave_v2.provider_routes
SET status = $4::TEXT,
    last_validated_at = $5::TIMESTAMPTZ,
    validated_secret_version = $6::BIGINT,
    validated_model = $7::TEXT,
    validation_evidence = $8::JSONB,
    revision = revision + 1
WHERE account_id = $1 AND namespace_id = $2 AND id = $3 AND revision = $9
RETURNING revision
`, req.Caller.AccountID, req.NamespaceID, target.RouteID, status, validatedAtValue,
			validatedVersionValue, validatedModelValue, validationBytes, target.RouteRevision).Scan(&nextRevision)
		if errors.Is(err, pgx.ErrNoRows) {
			return corev2.ErrProviderActivationStale
		}
		if err != nil {
			return fmt.Errorf("v2 postgres: record provider validation: %w", err)
		}
		if success {
			if _, err := db.Exec(txCtx, `
UPDATE kave_v2.secrets
SET last_validated_at = $5
WHERE account_id = $1 AND namespace_id = $2 AND id = $3 AND version = $4 AND status = 'active'
`, req.Caller.AccountID, req.NamespaceID, target.SecretID, target.SecretVersion, now); err != nil {
				return fmt.Errorf("v2 postgres: mark provider secret validated: %w", err)
			}
		}
		if _, err := db.Exec(txCtx, `
INSERT INTO kave_v2.audit_events (
    id, account_id, namespace_id, service_key_id, event, resource_type,
    resource_id, outcome, details, created_at
) VALUES ($1, $2, $3, $4, 'provider_route.activate', 'provider_route', $5, $6, $7, $8)
`, ids.New("aud"), req.Caller.AccountID, req.NamespaceID, req.Caller.ServiceKeyID,
			target.RouteID, outcome, auditDetailBytes, now); err != nil {
			return fmt.Errorf("v2 postgres: audit provider validation: %w", err)
		}
		result = corev2.ProviderRouteActivationResult{
			RouteID: target.RouteID, Route: target.Route, Provider: target.Provider, Model: target.Model,
			Status: status, RouteRevision: nextRevision, SecretVersion: target.SecretVersion,
			ProviderRequestID: evidence.ProviderRequestID,
		}
		if success {
			result.ValidatedAt = now
		}
		return nil
	})
	return result, err
}

func sanitizeProviderValidationEvidence(evidence corev2.ProviderRouteValidationEvidence) corev2.ProviderRouteValidationEvidence {
	if evidence.HTTPStatus < 100 || evidence.HTTPStatus > 599 {
		evidence.HTTPStatus = 0
	}
	evidence.ProviderRequestID = sanitizeProviderRequestID(evidence.ProviderRequestID)
	return evidence
}

func sanitizeProviderRequestID(value string) string {
	if len(value) > 255 || strings.ContainsAny(value, "\r\n") {
		return ""
	}
	return value
}

var _ corev2.ProviderRouteActivationStore = (*ProviderStore)(nil)
