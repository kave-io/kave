package casbin

import (
	"context"
	"log/slog"
	"time"

	casbinpkg "github.com/casbin/casbin/v3"
)

// AuditedAuthorization wraps a Casbin implementation with audit logging.
type AuditedAuthorization struct {
	inner  Casbin
	logger *slog.Logger
}

func NewAuditedAuthorization(inner Casbin, logger *slog.Logger) Casbin {
	if logger == nil {
		logger = slog.Default()
	}
	return &AuditedAuthorization{
		inner:  inner,
		logger: logger,
	}
}

func (a *AuditedAuthorization) Enforce(ctx context.Context, subject GroupSubject, domain Domain, object Resource, action Action) (bool, error) {
	start := time.Now()
	allowed, err := a.inner.Enforce(ctx, subject, domain, object, action)
	duration := time.Since(start)

	attrs := []any{
		"subject", string(subject),
		"domain", string(domain),
		"resource", string(object),
		"action", string(action),
		"allowed", allowed,
		"duration_ms", duration.Milliseconds(),
	}

	if err != nil {
		attrs = append(attrs, "error", err.Error())
		a.logger.Error("authz_decision", attrs...)
	} else if allowed {
		a.logger.Info("authz_decision", attrs...)
	} else {
		a.logger.Warn("authz_decision", attrs...)
	}

	return allowed, err
}

func (a *AuditedAuthorization) MustEnforce(ctx context.Context, subject GroupSubject, domain Domain, object Resource, action Action) error {
	ok, err := a.Enforce(ctx, subject, domain, object, action)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}

func (a *AuditedAuthorization) AddRoleForUserInDomain(ctx context.Context, subject GroupSubject, role Role, domain Domain) (bool, error) {
	added, err := a.inner.AddRoleForUserInDomain(ctx, subject, role, domain)

	attrs := []any{
		"operation", "add_role",
		"subject", string(subject),
		"role", string(role),
		"domain", string(domain),
		"added", added,
	}

	if err != nil {
		attrs = append(attrs, "error", err.Error())
		a.logger.Error("authz_role_change", attrs...)
	} else {
		a.logger.Info("authz_role_change", attrs...)
	}

	return added, err
}

func (a *AuditedAuthorization) RemoveRoleForUserInDomain(ctx context.Context, subject GroupSubject, role Role, domain Domain) (bool, error) {
	removed, err := a.inner.RemoveRoleForUserInDomain(ctx, subject, role, domain)

	attrs := []any{
		"operation", "remove_role",
		"subject", string(subject),
		"role", string(role),
		"domain", string(domain),
		"removed", removed,
	}

	if err != nil {
		attrs = append(attrs, "error", err.Error())
		a.logger.Error("authz_role_change", attrs...)
	} else {
		a.logger.Info("authz_role_change", attrs...)
	}

	return removed, err
}

func (a *AuditedAuthorization) GetRolesForUserInDomain(ctx context.Context, subject GroupSubject, domain Domain) ([]Role, error) {
	return a.inner.GetRolesForUserInDomain(ctx, subject, domain)
}

func (a *AuditedAuthorization) AddPermission(ctx context.Context, role Role, domain Domain, object Resource, action Action, effect PolicyEffect) (bool, error) {
	added, err := a.inner.AddPermission(ctx, role, domain, object, action, effect)

	attrs := []any{
		"operation", "add_permission",
		"role", string(role),
		"domain", string(domain),
		"resource", string(object),
		"action", string(action),
		"effect", string(effect),
		"added", added,
	}

	if err != nil {
		attrs = append(attrs, "error", err.Error())
		a.logger.Error("authz_permission_change", attrs...)
	} else {
		a.logger.Info("authz_permission_change", attrs...)
	}

	return added, err
}

func (a *AuditedAuthorization) RemovePermission(ctx context.Context, role Role, domain Domain, object Resource, action Action, effect PolicyEffect) (bool, error) {
	removed, err := a.inner.RemovePermission(ctx, role, domain, object, action, effect)

	attrs := []any{
		"operation", "remove_permission",
		"role", string(role),
		"domain", string(domain),
		"resource", string(object),
		"action", string(action),
		"effect", string(effect),
		"removed", removed,
	}

	if err != nil {
		attrs = append(attrs, "error", err.Error())
		a.logger.Error("authz_permission_change", attrs...)
	} else {
		a.logger.Info("authz_permission_change", attrs...)
	}

	return removed, err
}

func (a *AuditedAuthorization) Raw() *casbinpkg.DistributedEnforcer {
	return a.inner.Raw()
}
