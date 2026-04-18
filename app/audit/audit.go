package audit

import (
	"context"

	"github.com/kave-io/kave/core/store"
	auditv1 "github.com/kave-io/kave/proto/gen/kave/audit/v1"
	"google.golang.org/grpc"
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
	// TODO: implement
	return nil, nil
}

func (s *Server) QueryAudits(ctx context.Context, req *auditv1.QueryAuditsRequest) (*auditv1.QueryAuditsResponse, error) {
	// TODO: implement
	return nil, nil
}
