package connectport

import (
	"net/http"

	connect "connectrpc.com/connect"
	appaudit "github.com/kave-io/kave/server/app/audit"
	appcontrol "github.com/kave-io/kave/server/app/control"
	appruntime "github.com/kave-io/kave/server/app/runtime"
	auditv1connect "github.com/kave-io/kave/proto/gen/kave/audit/v1/auditv1connect"
	controlv1connect "github.com/kave-io/kave/proto/gen/kave/control/v1/controlv1connect"
	runtimev1connect "github.com/kave-io/kave/proto/gen/kave/runtime/v1/runtimev1connect"
)

// Register mounts browser-compatible Connect handlers under /rpc.
// It intentionally wires RPCs currently used by the dashboard.
func Register(mux *http.ServeMux, control *appcontrol.Server, runtime *appruntime.Server, audit *appaudit.Server) {
	mux.Handle(
		"/rpc"+controlv1connect.ControlPlaneServiceListAgentsProcedure,
		connect.NewUnaryHandlerSimple("/rpc"+controlv1connect.ControlPlaneServiceListAgentsProcedure, control.ListAgents),
	)
	mux.Handle(
		"/rpc"+controlv1connect.ControlPlaneServiceGetAgentProcedure,
		connect.NewUnaryHandlerSimple("/rpc"+controlv1connect.ControlPlaneServiceGetAgentProcedure, control.GetAgent),
	)
	mux.Handle(
		"/rpc"+controlv1connect.ControlPlaneServiceCreateAgentProcedure,
		connect.NewUnaryHandlerSimple("/rpc"+controlv1connect.ControlPlaneServiceCreateAgentProcedure, control.CreateAgent),
	)
	mux.Handle(
		"/rpc"+controlv1connect.ControlPlaneServiceUpdateAgentProcedure,
		connect.NewUnaryHandlerSimple("/rpc"+controlv1connect.ControlPlaneServiceUpdateAgentProcedure, control.UpdateAgent),
	)
	mux.Handle(
		"/rpc"+controlv1connect.ControlPlaneServiceGetPolicyProcedure,
		connect.NewUnaryHandlerSimple("/rpc"+controlv1connect.ControlPlaneServiceGetPolicyProcedure, control.GetPolicy),
	)
	mux.Handle(
		"/rpc"+controlv1connect.ControlPlaneServiceCreatePolicyProcedure,
		connect.NewUnaryHandlerSimple("/rpc"+controlv1connect.ControlPlaneServiceCreatePolicyProcedure, control.CreatePolicy),
	)

	mux.Handle(
		"/rpc"+runtimev1connect.RuntimeServiceListRunsProcedure,
		connect.NewUnaryHandlerSimple("/rpc"+runtimev1connect.RuntimeServiceListRunsProcedure, runtime.ListRuns),
	)
	mux.Handle(
		"/rpc"+runtimev1connect.RuntimeServiceGetRunProcedure,
		connect.NewUnaryHandlerSimple("/rpc"+runtimev1connect.RuntimeServiceGetRunProcedure, runtime.GetRun),
	)
	mux.Handle(
		"/rpc"+runtimev1connect.RuntimeServiceQuerySpansProcedure,
		connect.NewUnaryHandlerSimple("/rpc"+runtimev1connect.RuntimeServiceQuerySpansProcedure, runtime.QuerySpans),
	)
	mux.Handle(
		"/rpc"+runtimev1connect.RuntimeServiceGetSpendReportProcedure,
		connect.NewUnaryHandlerSimple("/rpc"+runtimev1connect.RuntimeServiceGetSpendReportProcedure, runtime.GetSpendReport),
	)
	mux.Handle(
		"/rpc"+runtimev1connect.RuntimeServiceGetPriceBookProcedure,
		connect.NewUnaryHandlerSimple("/rpc"+runtimev1connect.RuntimeServiceGetPriceBookProcedure, runtime.GetPriceBook),
	)

	mux.Handle(
		"/rpc"+auditv1connect.AuditServiceQueryAuditsProcedure,
		connect.NewUnaryHandlerSimple("/rpc"+auditv1connect.AuditServiceQueryAuditsProcedure, audit.QueryAudits),
	)
}
