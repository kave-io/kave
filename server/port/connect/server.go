package connectport

import (
	"context"
	"io"
	"net/http"

	connect "connectrpc.com/connect"
	auditv1 "github.com/kave-io/kave/proto/gen/kave/audit/v1"
	auditv1connect "github.com/kave-io/kave/proto/gen/kave/audit/v1/auditv1connect"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	controlv1connect "github.com/kave-io/kave/proto/gen/kave/control/v1/controlv1connect"
	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
	runtimev1connect "github.com/kave-io/kave/proto/gen/kave/runtime/v1/runtimev1connect"
	appaudit "github.com/kave-io/kave/server/app/audit"
	appcontrol "github.com/kave-io/kave/server/app/control"
	appruntime "github.com/kave-io/kave/server/app/runtime"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Register mounts browser-compatible Connect handlers under /rpc.
// It intentionally wires RPCs currently used by the dashboard.
func Register(mux *http.ServeMux, control *appcontrol.Server, runtime *appruntime.Server, audit *appaudit.Server, daemon *appcontrol.DaemonServiceImpl, auth AuthConfig) {
	authenticate := NewAuthMiddleware(auth)
	mount := func(procedure string, handler http.Handler) {
		mux.Handle("/rpc"+procedure, authenticate(http.StripPrefix("/rpc", handler)))
	}

	mountUnary := func(procedure string, handler http.Handler) {
		mount(procedure, handler)
	}

	// Control plane.
	mountUnary(controlv1connect.ControlPlaneServiceListProjectsProcedure, connect.NewUnaryHandlerSimple(controlv1connect.ControlPlaneServiceListProjectsProcedure, control.ListProjects))
	mountUnary(controlv1connect.ControlPlaneServiceListEnvironmentsProcedure, connect.NewUnaryHandlerSimple(controlv1connect.ControlPlaneServiceListEnvironmentsProcedure, control.ListEnvironments))
	mountUnary(controlv1connect.ControlPlaneServiceListAgentsProcedure, connect.NewUnaryHandlerSimple(controlv1connect.ControlPlaneServiceListAgentsProcedure, control.ListAgents))
	mountUnary(controlv1connect.ControlPlaneServiceGetAgentProcedure, connect.NewUnaryHandlerSimple(controlv1connect.ControlPlaneServiceGetAgentProcedure, control.GetAgent))
	mountUnary(controlv1connect.ControlPlaneServiceCreateAgentProcedure, connect.NewUnaryHandlerSimple(controlv1connect.ControlPlaneServiceCreateAgentProcedure, control.CreateAgent))
	mountUnary(controlv1connect.ControlPlaneServiceUpdateAgentProcedure, connect.NewUnaryHandlerSimple(controlv1connect.ControlPlaneServiceUpdateAgentProcedure, control.UpdateAgent))
	mountUnary(controlv1connect.ControlPlaneServiceGetPolicyProcedure, connect.NewUnaryHandlerSimple(controlv1connect.ControlPlaneServiceGetPolicyProcedure, control.GetPolicy))
	mountUnary(controlv1connect.ControlPlaneServiceListPoliciesProcedure, connect.NewUnaryHandlerSimple(controlv1connect.ControlPlaneServiceListPoliciesProcedure, control.ListPolicies))
	mountUnary(controlv1connect.ControlPlaneServiceCreatePolicyProcedure, connect.NewUnaryHandlerSimple(controlv1connect.ControlPlaneServiceCreatePolicyProcedure, control.CreatePolicy))
	mountUnary(controlv1connect.ControlPlaneServiceUpdatePolicyProcedure, connect.NewUnaryHandlerSimple(controlv1connect.ControlPlaneServiceUpdatePolicyProcedure, control.UpdatePolicy))
	mountUnary(controlv1connect.ControlPlaneServiceCreateTokenProcedure, connect.NewUnaryHandlerSimple(controlv1connect.ControlPlaneServiceCreateTokenProcedure, control.CreateToken))
	mountUnary(controlv1connect.ControlPlaneServiceListTokensProcedure, connect.NewUnaryHandlerSimple(controlv1connect.ControlPlaneServiceListTokensProcedure, control.ListTokens))
	mountUnary(controlv1connect.ControlPlaneServiceRevokeTokenProcedure, connect.NewUnaryHandlerSimple(controlv1connect.ControlPlaneServiceRevokeTokenProcedure, control.RevokeToken))
	mountUnary(controlv1connect.ControlPlaneServiceListCredentialsProcedure, connect.NewUnaryHandlerSimple(controlv1connect.ControlPlaneServiceListCredentialsProcedure, control.ListCredentials))

	// Runtime.
	mountUnary(runtimev1connect.RuntimeServiceListRunsProcedure, connect.NewUnaryHandlerSimple(runtimev1connect.RuntimeServiceListRunsProcedure, runtime.ListRuns))
	mountUnary(runtimev1connect.RuntimeServiceGetRunProcedure, connect.NewUnaryHandlerSimple(runtimev1connect.RuntimeServiceGetRunProcedure, runtime.GetRun))
	mountUnary(runtimev1connect.RuntimeServiceQuerySpansProcedure, connect.NewUnaryHandlerSimple(runtimev1connect.RuntimeServiceQuerySpansProcedure, runtime.QuerySpans))
	mountUnary(runtimev1connect.RuntimeServiceGetSpendReportProcedure, connect.NewUnaryHandlerSimple(runtimev1connect.RuntimeServiceGetSpendReportProcedure, runtime.GetSpendReport))
	mountUnary(runtimev1connect.RuntimeServiceGetPriceBookProcedure, connect.NewUnaryHandlerSimple(runtimev1connect.RuntimeServiceGetPriceBookProcedure, runtime.GetPriceBook))
	mountUnary(runtimev1connect.RuntimeServiceExportTraceProcedure, connect.NewUnaryHandlerSimple(runtimev1connect.RuntimeServiceExportTraceProcedure, runtime.ExportTrace))
	mountUnary(runtimev1connect.RuntimeServiceGetDashboardOverviewProcedure, connect.NewUnaryHandlerSimple(runtimev1connect.RuntimeServiceGetDashboardOverviewProcedure, runtime.GetDashboardOverview))
	mountUnary(runtimev1connect.RuntimeServiceGetTraceGraphProcedure, connect.NewUnaryHandlerSimple(runtimev1connect.RuntimeServiceGetTraceGraphProcedure, runtime.GetTraceGraph))
	mount(runtimev1connect.RuntimeServiceWatchRunsProcedure, connect.NewServerStreamHandlerSimple(runtimev1connect.RuntimeServiceWatchRunsProcedure, watchRuns(runtime)))
	mount(runtimev1connect.RuntimeServiceWatchEventsProcedure, connect.NewServerStreamHandlerSimple(runtimev1connect.RuntimeServiceWatchEventsProcedure, watchEvents(runtime)))
	mount(runtimev1connect.RuntimeServiceStreamSpansProcedure, connect.NewServerStreamHandlerSimple(runtimev1connect.RuntimeServiceStreamSpansProcedure, streamSpans(runtime)))

	// Audit.
	mountUnary(auditv1connect.AuditServiceQueryAuditsProcedure, connect.NewUnaryHandlerSimple(auditv1connect.AuditServiceQueryAuditsProcedure, audit.QueryAudits))

	// Daemon lifecycle/configuration.
	if daemon != nil {
		mountUnary(controlv1connect.DaemonServiceStatusProcedure, connect.NewUnaryHandlerSimple(controlv1connect.DaemonServiceStatusProcedure, daemon.Status))
		mountUnary(controlv1connect.DaemonServiceDoctorProcedure, connect.NewUnaryHandlerSimple(controlv1connect.DaemonServiceDoctorProcedure, daemon.Doctor))
		mountUnary(controlv1connect.DaemonServiceConfigViewProcedure, connect.NewUnaryHandlerSimple(controlv1connect.DaemonServiceConfigViewProcedure, daemon.ConfigView))
		mountUnary(controlv1connect.DaemonServiceConfigPathsProcedure, connect.NewUnaryHandlerSimple(controlv1connect.DaemonServiceConfigPathsProcedure, daemon.ConfigPaths))
		mountUnary(controlv1connect.DaemonServiceBuildPlanProcedure, connect.NewUnaryHandlerSimple(controlv1connect.DaemonServiceBuildPlanProcedure, daemon.BuildPlan))
		mountUnary(controlv1connect.DaemonServiceConfigReloadProcedure, connect.NewUnaryHandlerSimple(controlv1connect.DaemonServiceConfigReloadProcedure, daemon.ConfigReload))
	}
}

func watchRuns(runtime *appruntime.Server) func(context.Context, *runtimev1.WatchRunsRequest, *connect.ServerStream[runtimev1.RunRecord]) error {
	return func(ctx context.Context, req *runtimev1.WatchRunsRequest, stream *connect.ServerStream[runtimev1.RunRecord]) error {
		return runtime.WatchRuns(req, &serverStream[runtimev1.RunRecord]{ctx: ctx, stream: stream})
	}
}

func watchEvents(runtime *appruntime.Server) func(context.Context, *runtimev1.WatchEventsRequest, *connect.ServerStream[runtimev1.RuntimeEvent]) error {
	return func(ctx context.Context, req *runtimev1.WatchEventsRequest, stream *connect.ServerStream[runtimev1.RuntimeEvent]) error {
		return runtime.WatchEvents(req, &serverStream[runtimev1.RuntimeEvent]{ctx: ctx, stream: stream})
	}
}

func streamSpans(runtime *appruntime.Server) func(context.Context, *runtimev1.StreamSpansRequest, *connect.ServerStream[runtimev1.SpanEvent]) error {
	return func(ctx context.Context, req *runtimev1.StreamSpansRequest, stream *connect.ServerStream[runtimev1.SpanEvent]) error {
		return runtime.StreamSpans(req, &serverStream[runtimev1.SpanEvent]{ctx: ctx, stream: stream})
	}
}

type serverStream[T any] struct {
	ctx    context.Context
	stream *connect.ServerStream[T]
}

func (s *serverStream[T]) Send(msg *T) error            { return s.stream.Send(msg) }
func (s *serverStream[T]) Context() context.Context     { return s.ctx }
func (s *serverStream[T]) SetHeader(metadata.MD) error  { return nil }
func (s *serverStream[T]) SendHeader(metadata.MD) error { return nil }
func (s *serverStream[T]) SetTrailer(metadata.MD)       {}
func (s *serverStream[T]) SendMsg(m any) error {
	msg, ok := m.(*T)
	if !ok {
		return io.ErrUnexpectedEOF
	}
	return s.Send(msg)
}
func (s *serverStream[T]) RecvMsg(any) error { return io.EOF }

// Compile-time sanity checks for simple handlers used by this package.
var _ = (*emptypb.Empty)(nil)
var _ = (*controlv1.DaemonStatusResponse)(nil)
var _ = (*auditv1.QueryAuditsResponse)(nil)
