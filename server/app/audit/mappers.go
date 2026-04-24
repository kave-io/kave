package audit

import (
	"time"

	auditmodel "github.com/kave-io/kave/core/model/audit"
	"github.com/kave-io/kave/core/pkg/ids"
	auditv1 "github.com/kave-io/kave/proto/gen/kave/audit/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errInvalidAuditActorType = status.Error(codes.InvalidArgument, "actor_type is required")

func auditActorTypeFromProto(t auditv1.AuditActorType) (string, bool) {
	switch t {
	case auditv1.AuditActorType_AUDIT_ACTOR_TYPE_USER:
		return "user", true
	case auditv1.AuditActorType_AUDIT_ACTOR_TYPE_API_KEY:
		return "api_key", true
	case auditv1.AuditActorType_AUDIT_ACTOR_TYPE_SYSTEM:
		return "system", true
	default:
		return "", false
	}
}

func auditActorTypeToProto(s string) auditv1.AuditActorType {
	switch s {
	case "user":
		return auditv1.AuditActorType_AUDIT_ACTOR_TYPE_USER
	case "api_key":
		return auditv1.AuditActorType_AUDIT_ACTOR_TYPE_API_KEY
	case "system":
		return auditv1.AuditActorType_AUDIT_ACTOR_TYPE_SYSTEM
	default:
		return auditv1.AuditActorType_AUDIT_ACTOR_TYPE_UNSPECIFIED
	}
}

func auditLogFromEntry(entry *auditv1.AppendAuditInput) (*auditmodel.AuditLog, error) {
	actorType, ok := auditActorTypeFromProto(entry.GetActorType())
	if !ok {
		return nil, errInvalidAuditActorType
	}

	return &auditmodel.AuditLog{
		ID:           ids.New("aud"),
		OrgID:        entry.GetOrgId(),
		ProjectID:    stringPtrOrNil(entry.GetProjectId()),
		EnvID:        stringPtrOrNil(entry.GetEnvId()),
		ActorID:      entry.GetActorId(),
		ActorType:    actorType,
		Event:        entry.GetEvent(),
		ResourceType: entry.GetResourceType(),
		ResourceID:   entry.GetResourceId(),
		DiffBefore:   entry.GetDiffBefore(),
		DiffAfter:    entry.GetDiffAfter(),
		IP:           stringPtrOrNil(entry.GetIp()),
		Provenance:   entry.GetProvenance(),
		CreatedAt:    time.Now().UnixMilli(),
	}, nil
}

func auditLogToProto(entry *auditmodel.AuditLog) *auditv1.AuditLog {
	if entry == nil {
		return nil
	}
	return &auditv1.AuditLog{
		Id:           entry.ID,
		OrgId:        entry.OrgID,
		ProjectId:    entry.ProjectID,
		EnvId:        entry.EnvID,
		ActorId:      entry.ActorID,
		ActorType:    auditActorTypeToProto(entry.ActorType),
		Event:        entry.Event,
		ResourceType: entry.ResourceType,
		ResourceId:   entry.ResourceID,
		DiffBefore:   entry.DiffBefore,
		DiffAfter:    entry.DiffAfter,
		Ip:           entry.IP,
		CreatedAtMs:  entry.CreatedAt,
		Provenance:   entry.Provenance,
	}
}

func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	out := s
	return &out
}
