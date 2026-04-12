package mappers

import (
	"encoding/json"

	controlmodel "github.com/kave-io/kave/core/model/control"
)

// AuditEntryInput is the control-plane input for appending an audit log entry.
type AuditEntryInput struct {
	ID           string
	OrgID        string
	ProjectID    *string
	EnvID        *string
	ActorID      string
	ActorType    string
	Event        string
	ResourceType string
	ResourceID   string
	Before       any    // serialized as Diff.before; nil for creates
	After        any    // serialized as Diff.after; nil for deletes
	IP           *string
	CreatedAt    *int64
}

// AuditEntryToModel converts app-layer input to a controlmodel.AuditLog entry.
func AuditEntryToModel(in *AuditEntryInput) *controlmodel.AuditLog {
	if in == nil {
		return nil
	}

	now := msSinceEpoch()
	if in.CreatedAt != nil {
		now = *in.CreatedAt
	}

	diff := encodeDiff(in.Before, in.After)

	return &controlmodel.AuditLog{
		ID:           in.ID,
		OrgID:        in.OrgID,
		ProjectID:    in.ProjectID,
		EnvID:        in.EnvID,
		ActorID:      in.ActorID,
		ActorType:    in.ActorType,
		Event:        in.Event,
		ResourceType: in.ResourceType,
		ResourceID:   in.ResourceID,
		Diff:         diff,
		IP:           in.IP,
		CreatedAt:    now,
	}
}

// encodeDiff serializes before/after into a JSON diff blob.
func encodeDiff(before, after any) []byte {
	if before == nil && after == nil {
		return nil
	}
	payload := map[string]any{
		"before": before,
		"after":  after,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return b
}
