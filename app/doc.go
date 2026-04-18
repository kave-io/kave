// Package app implements the Kave control plane gRPC services.
//
// It binds together:
// - core: domain models, store interfaces, business logic
// - server: infra (postgres, crypto, paseto, casbin)
// - proto/gen: generated gRPC stubs
//
// Service layout mirrors proto:
// - control/: ControlPlaneAPI (agents, policies, credentials, etc.)
// - runtime/: RuntimeAPI (runs, actions, spans, cost)
// - audit/: AuditAPI (audit logs)
package app
