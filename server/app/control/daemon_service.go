package control

import (
	"context"

	"github.com/kave-io/kave/server/internal/daemon"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// DaemonServiceImpl implements controlv1.DaemonServiceServer.
type DaemonServiceImpl struct {
	controlv1.UnimplementedDaemonServiceServer

	state *daemon.State
}

// NewDaemonService creates a new DaemonService server.
func NewDaemonService(state *daemon.State) *DaemonServiceImpl {
	return &DaemonServiceImpl{
		state: state,
	}
}

// RegisterDaemonService registers the DaemonService with gRPC.
func (s *DaemonServiceImpl) RegisterDaemonService(srv *grpc.Server) {
	controlv1.RegisterDaemonServiceServer(srv, s)
}

func (s *DaemonServiceImpl) Status(ctx context.Context, _ *emptypb.Empty) (*controlv1.DaemonStatusResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *DaemonServiceImpl) Doctor(ctx context.Context, _ *emptypb.Empty) (*controlv1.DoctorReportResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *DaemonServiceImpl) ConfigView(ctx context.Context, _ *emptypb.Empty) (*controlv1.ConfigViewResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *DaemonServiceImpl) ConfigDiff(ctx context.Context, _ *emptypb.Empty) (*controlv1.ConfigDiffResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *DaemonServiceImpl) ConfigPaths(ctx context.Context, _ *emptypb.Empty) (*controlv1.ConfigPathsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *DaemonServiceImpl) BuildPlan(ctx context.Context, _ *emptypb.Empty) (*controlv1.ApplyPlanResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *DaemonServiceImpl) Apply(ctx context.Context, req *controlv1.ApplyRequest) (*controlv1.ApplyReportResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *DaemonServiceImpl) ConfigReload(ctx context.Context, _ *emptypb.Empty) (*controlv1.ConfigReloadReportResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}

func (s *DaemonServiceImpl) AdminStore(ctx context.Context, _ *emptypb.Empty) (*controlv1.AdminStoreReportResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not yet implemented")
}
