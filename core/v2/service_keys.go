package v2

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

var (
	ErrServiceKeyNotFound         = errors.New("kave v2: service key not found")
	ErrServiceKeyMaterialConflict = errors.New("kave v2: service-key material conflict")
)

type IssueServiceKeyRequest struct {
	Caller         Caller
	NamespaceID    Ref
	Name           Ref
	LookupPrefix   string
	SecretHash     []byte
	Operations     []Operation
	AllowedAgents  []Ref
	CanAssertScope bool
	ExpiresAt      *time.Time
	IdempotencyKey Ref
}

func (r IssueServiceKeyRequest) ValidateRequest() error {
	if err := r.Caller.AuthorizeControl(r.NamespaceID, OperationKeysManage); err != nil {
		return err
	}
	if err := r.Name.ValidateName("service_key.name", true); err != nil {
		return err
	}
	if err := r.IdempotencyKey.Validate("idempotency_key", true); err != nil {
		return err
	}
	if err := ValidateServiceKeyVerifier(r.LookupPrefix, r.SecretHash); err != nil {
		return err
	}
	if len(r.Operations) == 0 || len(r.Operations) > 8 {
		return invalid("service_key.operations", "must contain between one and eight operations")
	}
	for _, operation := range r.Operations {
		switch operation {
		case OperationConfigApply, OperationSecretsWrite, OperationKeysManage,
			OperationLimitsSync, OperationUsageRead, OperationAuditRead,
			OperationConsume, OperationInvoke:
		default:
			return invalid("service_key.operations", fmt.Sprintf("contains unsupported operation %q", operation))
		}
	}
	if len(r.AllowedAgents) > 64 {
		return invalid("service_key.allowed_agents", "must contain at most 64 agents")
	}
	for _, agent := range r.AllowedAgents {
		if err := agent.ValidateName("service_key.allowed_agent", true); err != nil {
			return err
		}
	}
	if (slices.Contains(r.Operations, OperationConsume) || slices.Contains(r.Operations, OperationInvoke)) && len(r.AllowedAgents) == 0 {
		return invalid("service_key.allowed_agents", "is required for consume or invoke")
	}
	return nil
}

type IssuedServiceKey struct {
	ID              string
	Name            Ref
	Prefix          string
	Operations      []Operation
	AllowedAgentIDs []Ref
	CanAssertScope  bool
	ExpiresAt       *time.Time
	CreatedAt       time.Time
	Status          string
	Created         bool
}

type RevokeServiceKeyRequest struct {
	Caller Caller
	ID     Ref
	Reason string
}

func (r RevokeServiceKeyRequest) ValidateRequest() error {
	if r.Caller.Bootstrap {
		return fmt.Errorf("%w: bootstrap credential cannot revoke namespace resources", ErrUnauthorized)
	}
	if err := r.Caller.AuthorizeControl(r.Caller.NamespaceID, OperationKeysManage); err != nil {
		return err
	}
	if err := r.ID.Validate("service_key.id", true); err != nil {
		return err
	}
	if len(r.Reason) > 256 || strings.ContainsAny(r.Reason, "\r\n") {
		return invalid("reason", "must be at most 256 bytes on one line")
	}
	return nil
}

type ServiceKeyStore interface {
	IssueServiceKey(context.Context, IssueServiceKeyRequest) (IssuedServiceKey, error)
	RevokeServiceKey(context.Context, RevokeServiceKeyRequest) error
}

type ServiceKeyService struct{ store ServiceKeyStore }

func NewServiceKeyService(store ServiceKeyStore) *ServiceKeyService {
	return &ServiceKeyService{store: store}
}

func (s *ServiceKeyService) Issue(ctx context.Context, req IssueServiceKeyRequest) (IssuedServiceKey, error) {
	if err := req.ValidateRequest(); err != nil {
		return IssuedServiceKey{}, err
	}
	if s == nil || s.store == nil {
		return IssuedServiceKey{}, errors.New("kave v2: service-key store unavailable")
	}
	return s.store.IssueServiceKey(ctx, req)
}

func (s *ServiceKeyService) Revoke(ctx context.Context, req RevokeServiceKeyRequest) error {
	if err := req.ValidateRequest(); err != nil {
		return err
	}
	if s == nil || s.store == nil {
		return errors.New("kave v2: service-key store unavailable")
	}
	return s.store.RevokeServiceKey(ctx, req)
}
