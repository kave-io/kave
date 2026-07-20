package postgres

import (
	"context"
	"fmt"

	corev2 "github.com/kave-io/kave/core/v2"
)

// limitPolicyOnlyChange reports whether a reconciliation changes only policy
// attached to an otherwise identical accounting identity. Keeping the same
// limit row for these changes is what keeps the active window's used and
// reserved counters authoritative, including reservations that settle after a
// control-plane update commits.
func limitPolicyOnlyChange(fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	for _, field := range fields {
		switch field {
		case "hard_cap", "soft_cap", "enabled":
		default:
			return false
		}
	}
	return true
}

func updateCurrentLimitPolicy(
	ctx context.Context,
	db DBTX,
	accountID, namespaceID corev2.Ref,
	limitID string,
	spec corev2.LimitSpec,
	sourceVersion *string,
) error {
	tag, err := db.Exec(ctx, `
UPDATE kave_v2.limits
SET hard_cap = $4,
    soft_cap = $5,
    enabled = $6,
    source_version = COALESCE($7::text, source_version),
    revision = revision + 1
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
  AND superseded_at IS NULL
`, accountID, namespaceID, limitID, spec.HardCap, spec.SoftCap, spec.Enabled, sourceVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("expected to update one current limit policy, updated %d", tag.RowsAffected())
	}
	return nil
}
