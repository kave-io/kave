package v2

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

var (
	ErrProviderRouteNotFound    = errors.New("kave v2: provider route not found")
	ErrProviderValidationFailed = errors.New("kave v2: provider route validation failed")
	ErrProviderActivationStale  = errors.New("kave v2: provider route changed during validation")
)

// ActivateProviderRouteRequest explicitly binds a live provider check to one
// persisted route, credential version, and allowed model. It intentionally
// carries no prompt or provider payload.
type ActivateProviderRouteRequest struct {
	Caller      Caller
	NamespaceID Ref
	Route       Ref
	Model       Ref
}

func (r ActivateProviderRouteRequest) Validate() error {
	if err := r.Caller.AuthorizeControl(r.NamespaceID, OperationConfigApply); err != nil {
		return err
	}
	if r.Caller.Bootstrap {
		return fmt.Errorf("%w: bootstrap credential cannot activate provider routes", ErrUnauthorized)
	}
	if err := r.Route.ValidateName("route", true); err != nil {
		return err
	}
	return r.Model.Validate("model", false)
}

// ProviderRouteValidationTarget is a short-lived, write-only validation grant.
// Credential must be cleared immediately after validation and must never be
// serialized, logged, audited, or persisted by a validator.
type ProviderRouteValidationTarget struct {
	AccountID     Ref
	NamespaceID   Ref
	RouteID       Ref
	Route         Ref
	RouteRevision int64
	Provider      Ref
	Protocol      string
	BaseURL       string
	Model         Ref
	SecretID      Ref
	SecretName    Ref
	SecretVersion int64
	Credential    []byte `json:"-"`
}

// ProviderRouteValidationEvidence is deliberately bounded metadata, not a
// provider response. It is safe to retain in the route and audit ledgers.
type ProviderRouteValidationEvidence struct {
	HTTPStatus        int
	ProviderRequestID string
}

type ProviderRouteActivationResult struct {
	RouteID           Ref
	Route             Ref
	Provider          Ref
	Model             Ref
	Status            string
	RouteRevision     int64
	SecretVersion     int64
	ValidatedAt       time.Time
	ProviderRequestID string
}

type ProviderRouteActivationStore interface {
	PrepareProviderRouteActivation(context.Context, ActivateProviderRouteRequest) (ProviderRouteValidationTarget, error)
	RecordProviderRouteValidation(context.Context, ActivateProviderRouteRequest, ProviderRouteValidationTarget, ProviderRouteValidationEvidence, bool) (ProviderRouteActivationResult, error)
}

type ProviderRouteValidator interface {
	ValidateProviderRoute(context.Context, ProviderRouteValidationTarget) (ProviderRouteValidationEvidence, error)
}

type ProviderRouteActivationService struct {
	store     ProviderRouteActivationStore
	validator ProviderRouteValidator
}

func NewProviderRouteActivationService(store ProviderRouteActivationStore, validator ProviderRouteValidator) *ProviderRouteActivationService {
	return &ProviderRouteActivationService{store: store, validator: validator}
}

func (s *ProviderRouteActivationService) Activate(ctx context.Context, req ActivateProviderRouteRequest) (ProviderRouteActivationResult, error) {
	if err := req.Validate(); err != nil {
		return ProviderRouteActivationResult{}, err
	}
	if s == nil || s.store == nil || s.validator == nil {
		return ProviderRouteActivationResult{}, errors.New("kave v2: provider route activation unavailable")
	}
	target, err := s.store.PrepareProviderRouteActivation(ctx, req)
	if err != nil {
		return ProviderRouteActivationResult{}, err
	}
	defer clear(target.Credential)
	evidence, validationErr := s.validator.ValidateProviderRoute(ctx, target)
	result, recordErr := s.store.RecordProviderRouteValidation(ctx, req, target, evidence, validationErr == nil)
	if recordErr != nil {
		return ProviderRouteActivationResult{}, recordErr
	}
	if validationErr != nil {
		return ProviderRouteActivationResult{}, ErrProviderValidationFailed
	}
	return result, nil
}

func (t ProviderRouteValidationTarget) Validate() error {
	for field, value := range map[string]Ref{
		"target.account_id": t.AccountID, "target.namespace_id": t.NamespaceID,
		"target.route_id": t.RouteID, "target.route": t.Route,
		"target.provider": t.Provider, "target.model": t.Model,
		"target.secret_id": t.SecretID, "target.secret_name": t.SecretName,
	} {
		if err := value.Validate(field, true); err != nil {
			return err
		}
	}
	if t.RouteRevision <= 0 || t.SecretVersion <= 0 {
		return invalid("target.revision", "route and secret revisions must be positive")
	}
	if t.Protocol == "" || t.BaseURL == "" || len(t.Credential) == 0 {
		return invalid("target", "protocol, base URL, and credential are required")
	}
	return nil
}

// ModelAllowed is kept here so persistence implementations can enforce model
// selection before any credential leaves the database transaction.
func ModelAllowed(model string, allowed []string) bool {
	return model != "" && slices.Contains(allowed, model)
}
