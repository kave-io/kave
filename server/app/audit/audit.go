package audit

import (
	"context"

	auditmodel "github.com/kave-io/kave/core/model/audit"
	"github.com/kave-io/kave/core/store"
	auditv1 "github.com/kave-io/kave/proto/gen/kave/audit/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements auditv1.AuditServiceServer.
type Server struct {
	auditv1.UnimplementedAuditServiceServer

	// Stores
	auditStore store.AuditStore
}

// New creates a new AuditAPI server.
func New(auditStore store.AuditStore) *Server {
	return &Server{
		auditStore: auditStore,
	}
}

// Register registers the AuditService server with gRPC.
func (s *Server) Register(srv *grpc.Server) {
	auditv1.RegisterAuditServiceServer(srv, s)
}

func (s *Server) AppendAudit(ctx context.Context, req *auditv1.AppendAuditRequest) (*auditv1.AuditLog, error) {
	if req == nil || req.GetEntry() == nil {
		return nil, status.Error(codes.InvalidArgument, "entry is required")
	}
	if s.auditStore == nil {
		return nil, status.Error(codes.FailedPrecondition, "audit store unavailable")
	}

	entry, err := auditLogFromEntry(req.GetEntry())
	if err != nil {
		return nil, err
	}
	if entry.OrgID == "" {
		return nil, status.Error(codes.InvalidArgument, "org_id is required")
	}
	if entry.ActorID == "" {
		return nil, status.Error(codes.InvalidArgument, "actor_id is required")
	}
	if entry.Event == "" {
		return nil, status.Error(codes.InvalidArgument, "event is required")
	}
	if entry.ResourceType == "" {
		return nil, status.Error(codes.InvalidArgument, "resource_type is required")
	}
	if entry.ResourceID == "" {
		return nil, status.Error(codes.InvalidArgument, "resource_id is required")
	}

	if err := s.auditStore.AppendAudit(ctx, entry); err != nil {
		return nil, err
	}
	return auditLogToProto(entry), nil
}

func (s *Server) QueryAudits(ctx context.Context, req *auditv1.QueryAuditsRequest) (*auditv1.QueryAuditsResponse, error) {
	if s.auditStore == nil {
		return nil, status.Error(codes.FailedPrecondition, "audit store unavailable")
	}

	var filter *auditmodel.AuditFilter
	if req != nil && req.GetFilter() != nil {
		f := req.GetFilter()
		filter = &auditmodel.AuditFilter{
			OrgID:        f.GetOrgId(),
			ProjectID:    f.GetProjectId(),
			EnvID:        f.GetEnvId(),
			ActorID:      f.GetActorId(),
			ResourceType: f.GetResourceType(),
			ResourceID:   f.GetResourceId(),
			Event:        f.GetEvent(),
			FromMs:       f.FromMs,
			ToMs:         f.ToMs,
		}
	}

	page := store.Page{}
	if req != nil {
		page = store.Page{Limit: int(req.GetLimit()), Cursor: req.GetPageToken()}
	}
	result, err := s.auditStore.QueryAudits(ctx, filter, page)
	if err != nil {
		return nil, err
	}

	resp := &auditv1.QueryAuditsResponse{NextPageToken: result.NextCursor}
	for _, entry := range result.Items {
		resp.Entries = append(resp.Entries, auditLogToProto(entry))
	}
	return resp, nil
}
