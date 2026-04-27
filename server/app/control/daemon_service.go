package control

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kave-io/kave/server/internal/daemon"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	"google.golang.org/grpc"
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
	snap := s.state.Snapshot(ctx)

	return &controlv1.DaemonStatusResponse{
		Pid:       fmt.Sprintf("%d", snap.PID),
		Version:   snap.Version,
		StartedAt: snap.StartedAtMS,
		Uptime:    fmt.Sprintf("%dms", snap.UptimeMS),
		Status:    snap.Stores,
	}, nil
}

func (s *DaemonServiceImpl) Doctor(ctx context.Context, _ *emptypb.Empty) (*controlv1.DoctorReportResponse, error) {
	report := s.state.Doctor(ctx)

	resp := &controlv1.DoctorReportResponse{}
	for _, check := range report.Checks {
		resp.Checks = append(resp.Checks, &controlv1.DoctorCheck{
			Name:    check.Name,
			Status:  check.Status,
			Detail:  check.Detail,
		})
	}
	return resp, nil
}

func (s *DaemonServiceImpl) ConfigView(ctx context.Context, _ *emptypb.Empty) (*controlv1.ConfigViewResponse, error) {
	cfg, err := s.state.ConfigView()
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	return &controlv1.ConfigViewResponse{
		Raw: string(data),
	}, nil
}

func (s *DaemonServiceImpl) ConfigDiff(ctx context.Context, _ *emptypb.Empty) (*controlv1.ConfigDiffResponse, error) {
	diff, err := s.state.ConfigDiff()
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(diff)
	if err != nil {
		return nil, err
	}

	return &controlv1.ConfigDiffResponse{
		Diff: string(data),
	}, nil
}

func (s *DaemonServiceImpl) ConfigPaths(ctx context.Context, _ *emptypb.Empty) (*controlv1.ConfigPathsResponse, error) {
	paths := s.state.ConfigPaths()

	return &controlv1.ConfigPathsResponse{
		ConfigDir:  string(paths["config_dir"]),
		ConfigFile: string(paths["config_file"]),
		DataDir:    string(paths["data_dir"]),
		LogFile:    string(paths["log_file"]),
		PidFile:    string(paths["pid_file"]),
	}, nil
}

func (s *DaemonServiceImpl) BuildPlan(ctx context.Context, _ *emptypb.Empty) (*controlv1.ApplyPlanResponse, error) {
	plan, err := s.state.BuildPlan(ctx)
	if err != nil {
		return nil, err
	}

	resp := &controlv1.ApplyPlanResponse{}
	for _, op := range plan.Creates {
		resp.Changes = append(resp.Changes, &controlv1.PlannedChange{
			ResourceType: op.Kind,
			ResourceId:   op.Name,
			Action:       "create",
			Summary:      "create " + op.Name,
		})
	}
	for _, op := range plan.Updates {
		resp.Changes = append(resp.Changes, &controlv1.PlannedChange{
			ResourceType: op.Kind,
			ResourceId:   op.Name,
			Action:       "update",
			Summary:      "update " + op.Name,
		})
	}
	for _, op := range plan.Deletes {
		resp.Changes = append(resp.Changes, &controlv1.PlannedChange{
			ResourceType: op.Kind,
			ResourceId:   op.Name,
			Action:       "delete",
			Summary:      "delete " + op.Name,
		})
	}

	return resp, nil
}

func (s *DaemonServiceImpl) Apply(ctx context.Context, req *controlv1.ApplyRequest) (*controlv1.ApplyReportResponse, error) {
	plan, err := s.state.BuildPlan(ctx)
	if err != nil {
		return nil, err
	}

	report, err := s.state.Apply(ctx, plan, req.Prune)
	if err != nil {
		return nil, err
	}

	return &controlv1.ApplyReportResponse{
		Created: int32(report.Created),
		Updated: int32(report.Updated),
		Deleted: int32(report.Deleted),
		Summary: fmt.Sprintf("%d created, %d updated, %d deleted", report.Created, report.Updated, report.Deleted),
	}, nil
}

func (s *DaemonServiceImpl) ConfigReload(ctx context.Context, _ *emptypb.Empty) (*controlv1.ConfigReloadReportResponse, error) {
	report, err := s.state.Reload(ctx)
	if daemon.IsInvalidConfig(err) {
		return &controlv1.ConfigReloadReportResponse{
			Success:         false,
			Message:         err.Error(),
			ResourcesLoaded: 0,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return &controlv1.ConfigReloadReportResponse{
		Success:         true,
		Message:         strings.Join(report.Applied, ", "),
		ResourcesLoaded: int32(len(report.Applied)),
	}, nil
}

func (s *DaemonServiceImpl) AdminStore(ctx context.Context, _ *emptypb.Empty) (*controlv1.AdminStoreReportResponse, error) {
	report, err := s.state.AdminStore(ctx)
	if err != nil {
		return nil, err
	}

	return &controlv1.AdminStoreReportResponse{
		DatabaseUrl: "",
		TablesCount: int64(len(report.App)),
		TotalSize:   "",
		Tables:      map[string]*controlv1.StoreTableInfo{},
	}, nil
}
