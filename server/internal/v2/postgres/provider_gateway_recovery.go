package postgres

import (
	"context"
	"fmt"
	"time"

	corev2 "github.com/kave-io/kave/core/v2"
	"github.com/kave-io/kave/server/internal/v2/provider"
)

type expiredProviderInvocation struct {
	id           string
	serviceKeyID corev2.Ref
}

// reconcileExpiredProviderAttempts settles a bounded batch of abandoned
// provider attempts inside the caller's already RLS-scoped transaction.
//
// FOR UPDATE SKIP LOCKED gives concurrent gateway processes exclusive
// ownership of each recovery without a global lock. A started attempt is
// charged at its conservative reservation; a never-started attempt is
// released. The invocation row remains locked until those counters and their
// immutable ledger evidence commit atomically.
func reconcileExpiredProviderAttempts(
	ctx context.Context,
	db DBTX,
	caller corev2.Caller,
	now time.Time,
	batchSize int,
) (int, error) {
	if batchSize <= 0 {
		return 0, nil
	}
	if batchSize > maxExpiredProviderRecoveriesPerAdmission {
		batchSize = maxExpiredProviderRecoveriesPerAdmission
	}

	rows, err := db.Query(ctx, `
SELECT id, service_key_id
FROM kave_v2.invocations
WHERE account_id = $1 AND namespace_id = $2
  AND kind = 'provider'
  AND status IN ('pending', 'admitted')
  AND lease_expires_at IS NOT NULL
  AND lease_expires_at <= $3
ORDER BY lease_expires_at, id
LIMIT $4
FOR UPDATE SKIP LOCKED
`, caller.AccountID, caller.NamespaceID, now, batchSize)
	if err != nil {
		return 0, fmt.Errorf("v2 postgres: select expired provider attempts: %w", err)
	}
	targets := make([]expiredProviderInvocation, 0, batchSize)
	for rows.Next() {
		var target expiredProviderInvocation
		if err := rows.Scan(&target.id, &target.serviceKeyID); err != nil {
			rows.Close()
			return 0, fmt.Errorf("v2 postgres: scan expired provider attempt: %w", err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("v2 postgres: iterate expired provider attempts: %w", err)
	}
	rows.Close()

	for _, target := range targets {
		recovery := provider.BeginRequest{Caller: corev2.Caller{
			AccountID:    caller.AccountID,
			NamespaceID:  caller.NamespaceID,
			ServiceKeyID: target.serviceKeyID,
		}}
		if err := recoverExpiredProviderAttempt(ctx, db, recovery, target.id, now); err != nil {
			return 0, err
		}
	}
	return len(targets), nil
}
