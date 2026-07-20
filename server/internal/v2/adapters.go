package v2

import (
	"context"
	"errors"
	"fmt"

	corev2 "github.com/kave-io/kave/core/v2"
	v2postgres "github.com/kave-io/kave/server/internal/v2/postgres"
)

type serviceKeyStoreAdapter struct{ admin *v2postgres.ServiceKeyAdmin }

func (a serviceKeyStoreAdapter) IssueServiceKey(ctx context.Context, req corev2.IssueServiceKeyRequest) (corev2.IssuedServiceKey, error) {
	if a.admin == nil {
		return corev2.IssuedServiceKey{}, errors.New("v2 kernel: service-key admin unavailable")
	}
	actingServiceKeyID := req.Caller.ServiceKeyID
	if req.Caller.Bootstrap {
		actingServiceKeyID = ""
	}
	issued, err := a.admin.Issue(ctx, v2postgres.IssueServiceKeyRequest{
		Scope: v2postgres.Scope{
			AccountID: string(req.Caller.AccountID), NamespaceID: string(req.NamespaceID),
		},
		ActingServiceKeyID: actingServiceKeyID,
		IdempotencyKey:     req.IdempotencyKey,
		Name:               string(req.Name),
		LookupPrefix:       req.LookupPrefix,
		SecretHash:         append([]byte(nil), req.SecretHash...),
		Operations:         req.Operations,
		AllowedAgentNames:  req.AllowedAgents,
		CanAssertScope:     req.CanAssertScope,
		ExpiresAt:          req.ExpiresAt,
	})
	if errors.Is(err, v2postgres.ErrServiceKeyConflict) {
		return corev2.IssuedServiceKey{}, &corev2.IdempotencyConflictError{Key: req.IdempotencyKey}
	}
	if errors.Is(err, v2postgres.ErrServiceKeyMaterialConflict) {
		return corev2.IssuedServiceKey{}, corev2.ErrServiceKeyMaterialConflict
	}
	if err != nil {
		return corev2.IssuedServiceKey{}, err
	}
	agentIDs := make([]corev2.Ref, len(issued.AllowedAgentIDs))
	for i, id := range issued.AllowedAgentIDs {
		agentIDs[i] = corev2.Ref(id)
	}
	return corev2.IssuedServiceKey{
		ID: issued.ID, Name: corev2.Ref(issued.Name), Prefix: issued.Prefix, Operations: issued.Operations,
		AllowedAgentIDs: agentIDs, CanAssertScope: issued.CanAssertScope,
		ExpiresAt: issued.ExpiresAt, CreatedAt: issued.CreatedAt, Status: issued.Status, Created: issued.Created,
	}, nil
}

func (a serviceKeyStoreAdapter) RevokeServiceKey(ctx context.Context, req corev2.RevokeServiceKeyRequest) error {
	if a.admin == nil {
		return errors.New("v2 kernel: service-key admin unavailable")
	}
	_, err := a.admin.Revoke(ctx, v2postgres.RevokeServiceKeyRequest{
		Scope: v2postgres.Scope{
			AccountID: string(req.Caller.AccountID), NamespaceID: string(req.Caller.NamespaceID),
		},
		ActingServiceKeyID: req.Caller.ServiceKeyID,
		ServiceKeyID:       req.ID,
		Reason:             req.Reason,
	})
	if errors.Is(err, v2postgres.ErrServiceKeyNotFound) {
		return fmt.Errorf("%w: %s", corev2.ErrServiceKeyNotFound, req.ID)
	}
	return err
}

var _ corev2.ServiceKeyStore = serviceKeyStoreAdapter{}
