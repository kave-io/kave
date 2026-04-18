package mappers

import (
	"encoding/json"

	auditmodel "github.com/kave-io/kave/core/model/audit"
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
	Before       any // serialized as DiffBefore; nil for creates
	After        any // serialized as DiffAfter; nil for deletes
	Provenance   any // serialized as Provenance; nil when no FX/display provenance applies
	IP           *string
	CreatedAt    *int64
}

// AuditEntryToModel converts app-layer input to an auditmodel.AuditLog entry.
func AuditEntryToModel(in *AuditEntryInput) *auditmodel.AuditLog {
	if in == nil {
		return nil
	}

	now := msSinceEpoch()
	if in.CreatedAt != nil {
		now = *in.CreatedAt
	}

	return &auditmodel.AuditLog{
		ID:           in.ID,
		OrgID:        in.OrgID,
		ProjectID:    in.ProjectID,
		EnvID:        in.EnvID,
		ActorID:      in.ActorID,
		ActorType:    in.ActorType,
		Event:        in.Event,
		ResourceType: in.ResourceType,
		ResourceID:   in.ResourceID,
		DiffBefore:   encodeJSON(in.Before),
		DiffAfter:    encodeJSON(in.After),
		Provenance:   encodeJSON(in.Provenance),
		IP:           in.IP,
		CreatedAt:    now,
	}
}

func encodeJSON(v any) []byte {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
