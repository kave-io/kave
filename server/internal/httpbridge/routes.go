package httpbridge

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"time"

	appcontrol "github.com/kave-io/kave/app/control"
	appruntime "github.com/kave-io/kave/app/runtime"
	"github.com/kave-io/kave/core/mappers"
	controlmodel "github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/ids"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
	"github.com/kave-io/kave/server/internal/contract"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// BuildRoutes returns the HTTP routes served by the bridge.
func BuildRoutes(control *appcontrol.Server, runtime *appruntime.Server, app store.AppStore, spans store.SpanStore) []Route {
	return []Route{
		// Org / project / environment
		{Path: "GET /api/v1/orgs", Invoke: listOrgs(control)},
		{Path: "GET /api/v1/orgs/{id}", PathParams: []string{"id"}, Invoke: getOrg(control)},
		{Path: "POST /api/v1/orgs", Invoke: createOrg(control)},
		{Path: "GET /api/v1/projects", Invoke: listProjects(control)},
		{Path: "GET /api/v1/projects/{id}", PathParams: []string{"id"}, Invoke: getProject(control)},
		{Path: "POST /api/v1/projects", Invoke: createProject(control)},
		{Path: "GET /api/v1/environments", Invoke: listEnvironments(control)},
		{Path: "GET /api/v1/environments/{id}", PathParams: []string{"id"}, Invoke: getEnvironment(control)},
		{Path: "POST /api/v1/environments", Invoke: createEnvironment(control)},

		// Tokens / credentials / budgets
		{Path: "GET /api/v1/tokens", Invoke: listTokens(control)},
		{Path: "GET /api/v1/tokens/{id}", PathParams: []string{"id"}, Invoke: getToken(control)},
		{Path: "POST /api/v1/tokens", Invoke: createToken(control)},
		{Path: "DELETE /api/v1/tokens/{id}", PathParams: []string{"id"}, Invoke: revokeToken(control)},
		{Path: "GET /api/v1/credentials", Invoke: listCredentials(control)},
		{Path: "GET /api/v1/credentials/{id}", PathParams: []string{"id"}, Invoke: getCredential(control)},
		{Path: "POST /api/v1/credentials", Invoke: createCredential(control)},
		{Path: "PATCH /api/v1/credentials/{id}", PathParams: []string{"id"}, Invoke: updateCredential(control)},
		{Path: "POST /api/v1/credentials/{id}/rotate", PathParams: []string{"id"}, Invoke: rotateCredential(control)},
		{Path: "DELETE /api/v1/credentials/{id}", PathParams: []string{"id"}, Invoke: deleteCredential(control)},
		{Path: "POST /api/v1/credentials/{id}/revoke", PathParams: []string{"id"}, Invoke: revokeCredential(control)},
		{Path: "GET /api/v1/budgets/{agent_id}", PathParams: []string{"agent_id"}, Invoke: getBudget(control)},
		{Path: "POST /api/v1/budgets", Invoke: createBudget(control)},
		{Path: "DELETE /api/v1/budgets/{agent_id}", PathParams: []string{"agent_id"}, Invoke: deleteBudget(control)},

		// Runs / actions / spans
		{Path: "POST /api/v1/runs", Invoke: createRun(runtime)},
		{Path: "PATCH /api/v1/runs/{id}", PathParams: []string{"id"}, Invoke: updateRun(runtime)},
		{Path: "POST /api/v1/runs/{id}/cancel", PathParams: []string{"id"}, Invoke: cancelRun(runtime)},
		{Path: "POST /api/v1/actions", Invoke: createAction(runtime)},
		{Path: "GET /api/v1/actions/{id}", PathParams: []string{"id"}, Invoke: getAction(runtime)},
		{Path: "GET /api/v1/actions", Invoke: listActions(runtime)},
		{Path: "POST /api/v1/spans", Invoke: openSpan(runtime)},
		{Path: "POST /api/v1/spans/{span_id}/close", PathParams: []string{"span_id"}, Invoke: closeSpan(runtime)},
		{Path: "GET /api/v1/spans/{id}", PathParams: []string{"id"}, Invoke: getSpan(runtime)},

		// Agents
		{Path: "GET /api/v1/agents", Invoke: listAgents(app)},
		{Path: "GET /api/v1/agents/{id}", PathParams: []string{"id"}, Invoke: getAgent(app)},
		{Path: "POST /api/v1/agents", Invoke: createAgent(app)},
		{Path: "PATCH /api/v1/agents/{id}", PathParams: []string{"id"}, Invoke: updateAgent(app)},

		// Policies
		{Path: "GET /api/v1/policies", Invoke: listPolicies(app)},
		{Path: "GET /api/v1/policies/{id}", PathParams: []string{"id"}, Invoke: getPolicy(app)},
		{Path: "POST /api/v1/policies", Invoke: createPolicy(app)},

		// Runs
		{Path: "GET /api/v1/runs", Invoke: listRuns(app)},
		{Path: "GET /api/v1/runs/{id}", PathParams: []string{"id"}, Invoke: getRun(app)},
		{Path: "GET /api/v1/runs/{id}/spans", PathParams: []string{"id"}, Invoke: getRunSpans(spans)},

		// Spans
		{Path: "GET /api/v1/spans", Invoke: listSpans(spans)},

		// Cost and pricing
		{Path: "GET /api/v1/cost/summary", Invoke: getCostSummary(app)},
		{Path: "GET /api/v1/settings/pricing", Invoke: getPriceBook(app)},
		{Path: "PUT /api/v1/settings/pricing", Invoke: updatePriceBook(app)},

		// Unimplemented CLI-facing endpoints.
		{Path: "GET /api/v1/trace/tail", Invoke: unimplementedRoute("server.unimplemented")},
		{Path: "GET /api/v1/events/tail", Invoke: unimplementedRoute("server.unimplemented")},
		{Path: "GET /api/v1/logs/tail", Invoke: unimplementedRoute("server.unimplemented")},
		{Path: "GET /api/v1/status", Invoke: unimplementedRoute("server.unimplemented")},
		{Path: "GET /api/v1/doctor", Invoke: unimplementedRoute("server.unimplemented")},
		{Path: "GET /api/v1/config/view", Invoke: unimplementedRoute("server.unimplemented")},
		{Path: "POST /api/v1/config/reload", Invoke: unimplementedRoute("server.unimplemented")},
		{Path: "GET /api/v1/admin/store", Invoke: unimplementedRoute("server.unimplemented")},
		{Path: "POST /api/v1/apply", Invoke: unimplementedRoute("server.unimplemented")},
		{Path: "GET /api/v1/diff", Invoke: unimplementedRoute("server.unimplemented")},
		{Path: "GET /api/v1/watch", Invoke: unimplementedRoute("server.unimplemented")},
	}
}

func listOrgs(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, query url.Values, _ map[string]string) (Outcome, error) {
		limit, cursor := pageFromQuery(query)
		resp, err := control.ListOrganizations(ctx, &controlv1.ListOrganizationsRequest{Limit: int32(limit), Cursor: cursor})
		if err != nil {
			return Outcome{Kind: "OrganizationList"}, err
		}
		return Outcome{Kind: "OrganizationList", Data: resp.Organizations, Page: pageContract(limit, resp.NextCursor)}, nil
	}
}

func getOrg(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		resp, err := control.GetOrganization(ctx, &controlv1.GetOrganizationRequest{Id: path["id"]})
		if err != nil {
			return Outcome{Kind: "Organization"}, err
		}
		return Outcome{Kind: "Organization", Data: resp}, nil
	}
}

func createOrg(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		req := &controlv1.CreateOrganizationRequest{}
		if err := decodeProtoJSON(body, req); err != nil {
			return Outcome{Kind: "Organization"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		resp, err := control.CreateOrganization(ctx, req)
		if err != nil {
			return Outcome{Kind: "Organization"}, err
		}
		return Outcome{Kind: "Organization", Data: resp, Status: 201}, nil
	}
}

func listProjects(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, query url.Values, _ map[string]string) (Outcome, error) {
		limit, cursor := pageFromQuery(query)
		resp, err := control.ListProjects(ctx, &controlv1.ListProjectsRequest{OrgId: query.Get("org_id"), Limit: int32(limit), Cursor: cursor})
		if err != nil {
			return Outcome{Kind: "ProjectList"}, err
		}
		return Outcome{Kind: "ProjectList", Data: resp.Projects, Page: pageContract(limit, resp.NextCursor)}, nil
	}
}

func getProject(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		resp, err := control.GetProject(ctx, &controlv1.GetProjectRequest{Id: path["id"]})
		if err != nil {
			return Outcome{Kind: "Project"}, err
		}
		return Outcome{Kind: "Project", Data: resp}, nil
	}
}

func createProject(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		req := &controlv1.CreateProjectRequest{}
		if err := decodeProtoJSON(body, req); err != nil {
			return Outcome{Kind: "Project"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		resp, err := control.CreateProject(ctx, req)
		if err != nil {
			return Outcome{Kind: "Project"}, err
		}
		return Outcome{Kind: "Project", Data: resp, Status: 201}, nil
	}
}

func listEnvironments(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, query url.Values, _ map[string]string) (Outcome, error) {
		limit, cursor := pageFromQuery(query)
		resp, err := control.ListEnvironments(ctx, &controlv1.ListEnvironmentsRequest{ProjectId: query.Get("project_id"), Limit: int32(limit), Cursor: cursor})
		if err != nil {
			return Outcome{Kind: "EnvironmentList"}, err
		}
		return Outcome{Kind: "EnvironmentList", Data: resp.Environments, Page: pageContract(limit, resp.NextCursor)}, nil
	}
}

func getEnvironment(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		resp, err := control.GetEnvironment(ctx, &controlv1.GetEnvironmentRequest{Id: path["id"]})
		if err != nil {
			return Outcome{Kind: "Environment"}, err
		}
		return Outcome{Kind: "Environment", Data: resp}, nil
	}
}

func createEnvironment(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		req := &controlv1.CreateEnvironmentRequest{}
		if err := decodeProtoJSON(body, req); err != nil {
			return Outcome{Kind: "Environment"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		resp, err := control.CreateEnvironment(ctx, req)
		if err != nil {
			return Outcome{Kind: "Environment"}, err
		}
		return Outcome{Kind: "Environment", Data: resp, Status: 201}, nil
	}
}

func listTokens(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, query url.Values, _ map[string]string) (Outcome, error) {
		limit, cursor := pageFromQuery(query)
		resp, err := control.ListTokens(ctx, &controlv1.ListTokensRequest{AgentId: query.Get("agent_id"), Limit: int32(limit), Cursor: cursor})
		if err != nil {
			return Outcome{Kind: "TokenList"}, err
		}
		return Outcome{Kind: "TokenList", Data: resp.Tokens, Page: pageContract(limit, resp.NextCursor)}, nil
	}
}

func getToken(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		resp, err := control.GetToken(ctx, &controlv1.GetTokenRequest{Id: path["id"]})
		if err != nil {
			return Outcome{Kind: "Token"}, err
		}
		return Outcome{Kind: "Token", Data: resp}, nil
	}
}

func createToken(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		req := &controlv1.CreateTokenRequest{}
		if err := decodeProtoJSON(body, req); err != nil {
			return Outcome{Kind: "Token"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		resp, err := control.CreateToken(ctx, req)
		if err != nil {
			return Outcome{Kind: "Token"}, err
		}
		return Outcome{Kind: "Token", Data: resp, Status: 201}, nil
	}
}

func revokeToken(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		_, err := control.RevokeToken(ctx, &controlv1.RevokeTokenRequest{Id: path["id"]})
		if err != nil {
			return Outcome{Kind: "Token"}, err
		}
		return Outcome{Kind: "Token", Data: map[string]any{"status": "ok"}}, nil
	}
}

func listCredentials(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, query url.Values, _ map[string]string) (Outcome, error) {
		limit, cursor := pageFromQuery(query)
		filter := &controlv1.CredentialFilter{EnvId: query.Get("env_id"), ConnectorType: query.Get("connector_type"), AccountId: query.Get("account_id"), Label: query.Get("label")}
		resp, err := control.ListCredentials(ctx, &controlv1.ListCredentialsRequest{Filter: filter, Limit: int32(limit), Cursor: cursor})
		if err != nil {
			return Outcome{Kind: "CredentialList"}, err
		}
		return Outcome{Kind: "CredentialList", Data: resp.Credentials, Page: pageContract(limit, resp.NextCursor)}, nil
	}
}

func getCredential(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		resp, err := control.GetCredential(ctx, &controlv1.GetCredentialRequest{Id: path["id"]})
		if err != nil {
			return Outcome{Kind: "Credential"}, err
		}
		return Outcome{Kind: "Credential", Data: resp}, nil
	}
}

func createCredential(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		req := &controlv1.CreateCredentialRequest{}
		if err := decodeProtoJSON(body, req); err != nil {
			return Outcome{Kind: "Credential"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		resp, err := control.CreateCredential(ctx, req)
		if err != nil {
			return Outcome{Kind: "Credential"}, err
		}
		return Outcome{Kind: "Credential", Data: resp, Status: 201}, nil
	}
}

func updateCredential(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, path map[string]string) (Outcome, error) {
		req := &controlv1.UpdateCredentialRequest{Id: path["id"]}
		if err := decodeProtoJSON(body, req); err != nil {
			return Outcome{Kind: "Credential"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		resp, err := control.UpdateCredential(ctx, req)
		if err != nil {
			return Outcome{Kind: "Credential"}, err
		}
		return Outcome{Kind: "Credential", Data: resp}, nil
	}
}

func rotateCredential(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, path map[string]string) (Outcome, error) {
		req := &controlv1.RotateCredentialRequest{Id: path["id"]}
		if err := decodeProtoJSON(body, req); err != nil {
			return Outcome{Kind: "Credential"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		resp, err := control.RotateCredential(ctx, req)
		if err != nil {
			return Outcome{Kind: "Credential"}, err
		}
		return Outcome{Kind: "Credential", Data: resp}, nil
	}
}

func revokeCredential(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, path map[string]string) (Outcome, error) {
		req := &controlv1.RevokeCredentialRequest{Id: path["id"]}
		if err := decodeProtoJSON(body, req); err != nil {
			return Outcome{Kind: "Credential"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		_, err := control.RevokeCredential(ctx, req)
		if err != nil {
			return Outcome{Kind: "Credential"}, err
		}
		return Outcome{Kind: "Credential", Data: map[string]any{"status": "ok"}}, nil
	}
}

func deleteCredential(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		_, err := control.DeleteCredential(ctx, &controlv1.DeleteCredentialRequest{Id: path["id"]})
		if err != nil {
			return Outcome{Kind: "Credential"}, err
		}
		return Outcome{Kind: "Credential", Data: map[string]any{"status": "ok"}}, nil
	}
}

func getBudget(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		resp, err := control.GetBudget(ctx, &controlv1.GetBudgetRequest{AgentId: path["agent_id"]})
		if err != nil {
			return Outcome{Kind: "Budget"}, err
		}
		return Outcome{Kind: "Budget", Data: resp}, nil
	}
}

func createBudget(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		req := &controlv1.CreateBudgetRequest{}
		if err := decodeProtoJSON(body, req); err != nil {
			return Outcome{Kind: "Budget"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		resp, err := control.CreateBudget(ctx, req)
		if err != nil {
			return Outcome{Kind: "Budget"}, err
		}
		return Outcome{Kind: "Budget", Data: resp, Status: 201}, nil
	}
}

func deleteBudget(control *appcontrol.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		_, err := control.DeleteBudget(ctx, &controlv1.DeleteBudgetRequest{AgentId: path["agent_id"]})
		if err != nil {
			return Outcome{Kind: "Budget"}, err
		}
		return Outcome{Kind: "Budget", Data: map[string]any{"status": "ok"}}, nil
	}
}

func createRun(runtime *appruntime.Server) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		req := &runtimev1.CreateRunRequest{}
		if err := decodeProtoJSON(body, req); err != nil {
			return Outcome{Kind: "Run"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		resp, err := runtime.CreateRun(ctx, req)
		if err != nil {
			return Outcome{Kind: "Run"}, err
		}
		return Outcome{Kind: "Run", Data: resp, Status: 201}, nil
	}
}

func updateRun(runtime *appruntime.Server) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, path map[string]string) (Outcome, error) {
		req := &runtimev1.UpdateRunRequest{Id: path["id"]}
		if err := decodeProtoJSON(body, req); err != nil {
			return Outcome{Kind: "Run"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		resp, err := runtime.UpdateRun(ctx, req)
		if err != nil {
			return Outcome{Kind: "Run"}, err
		}
		return Outcome{Kind: "Run", Data: resp}, nil
	}
}

func cancelRun(runtime *appruntime.Server) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, path map[string]string) (Outcome, error) {
		req := &runtimev1.CancelRunRequest{Id: path["id"]}
		if err := decodeProtoJSON(body, req); err != nil {
			return Outcome{Kind: "Run"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		resp, err := runtime.CancelRun(ctx, req)
		if err != nil {
			return Outcome{Kind: "Run"}, err
		}
		return Outcome{Kind: "Run", Data: resp}, nil
	}
}

func createAction(runtime *appruntime.Server) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		req := &runtimev1.CreateActionRequest{}
		if err := decodeProtoJSON(body, req); err != nil {
			return Outcome{Kind: "Action"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		resp, err := runtime.CreateAction(ctx, req)
		if err != nil {
			return Outcome{Kind: "Action"}, err
		}
		return Outcome{Kind: "Action", Data: resp, Status: 201}, nil
	}
}

func getAction(runtime *appruntime.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		resp, err := runtime.GetAction(ctx, &runtimev1.GetActionRequest{Id: path["id"]})
		if err != nil {
			return Outcome{Kind: "Action"}, err
		}
		return Outcome{Kind: "Action", Data: resp}, nil
	}
}

func listActions(runtime *appruntime.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, query url.Values, _ map[string]string) (Outcome, error) {
		limit, cursor := pageFromQuery(query)
		req := &runtimev1.ListActionsRequest{Limit: int32(limit), Cursor: cursor}
		if runID := query.Get("run_id"); runID != "" {
			req.Filter = &runtimev1.ActionFilter{RunId: runID}
		}
		resp, err := runtime.ListActions(ctx, req)
		if err != nil {
			return Outcome{Kind: "ActionList"}, err
		}
		return Outcome{Kind: "ActionList", Data: resp.Actions, Page: pageContract(limit, resp.NextCursor)}, nil
	}
}

func openSpan(runtime *appruntime.Server) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		req := &runtimev1.OpenSpanRequest{}
		if err := decodeProtoJSON(body, req); err != nil {
			return Outcome{Kind: "Span"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		resp, err := runtime.OpenSpan(ctx, req)
		if err != nil {
			return Outcome{Kind: "Span"}, err
		}
		return Outcome{Kind: "Span", Data: resp, Status: 201}, nil
	}
}

func closeSpan(runtime *appruntime.Server) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, path map[string]string) (Outcome, error) {
		req := &runtimev1.CloseSpanRequest{SpanId: path["span_id"]}
		if err := decodeProtoJSON(body, req); err != nil {
			return Outcome{Kind: "Span"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		resp, err := runtime.CloseSpan(ctx, req)
		if err != nil {
			return Outcome{Kind: "Span"}, err
		}
		return Outcome{Kind: "Span", Data: resp}, nil
	}
}

func getSpan(runtime *appruntime.Server) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		resp, err := runtime.GetSpan(ctx, &runtimev1.GetSpanRequest{Id: path["id"]})
		if err != nil {
			return Outcome{Kind: "Span"}, err
		}
		return Outcome{Kind: "Span", Data: resp}, nil
	}
}

func decodeProtoJSON(body []byte, msg proto.Message) error {
	if len(body) == 0 {
		return nil
	}
	return protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(body, msg)
}

func listAgents(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, query url.Values, _ map[string]string) (Outcome, error) {
		envID := query.Get("env_id")
		if envID == "" {
			return Outcome{}, status.Error(codes.InvalidArgument, "env_id query parameter required")
		}
		limit, cursor := pageFromQuery(query)
		result, err := app.ListAgents(ctx, envID, store.Page{Limit: limit, Cursor: cursor})
		if err != nil {
			return Outcome{}, err
		}
		data := make([]*Agent, 0, len(result.Items))
		for _, agent := range result.Items {
			data = append(data, MapAgentToAPI(agent))
		}
		return Outcome{Kind: "AgentList", Data: data, Page: pageContract(limit, result.NextCursor)}, nil
	}
}

func getAgent(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		agent, err := app.GetAgentByID(ctx, path["id"])
		if err != nil {
			return Outcome{Kind: "Agent"}, err
		}
		if agent == nil {
			return Outcome{Kind: "Agent"}, status.Errorf(codes.NotFound, "agent %q not found", path["id"])
		}
		return Outcome{Kind: "Agent", Data: MapAgentToAPI(agent)}, nil
	}
}

func createAgent(app store.AppStore) InvokeFn {
	type request struct {
		ProjectID     string         `json:"project_id"`
		EnvID         string         `json:"env_id"`
		Name          string         `json:"name"`
		Description   *string        `json:"description,omitempty"`
		PolicyID      *string        `json:"policy_id,omitempty"`
		MonthlyBudget *string        `json:"monthly_budget,omitempty"`
		Metadata      map[string]any `json:"metadata,omitempty"`
	}

	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		var req request
		if err := json.Unmarshal(body, &req); err != nil {
			return Outcome{Kind: "Agent"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		if req.Name == "" || req.ProjectID == "" || req.EnvID == "" {
			return Outcome{Kind: "Agent"}, status.Error(codes.InvalidArgument, "name, project_id, and env_id required")
		}

		now := timeNowMS()
		agent := &controlmodel.Agent{
			ID:          generateID("agn"),
			ProjectID:   req.ProjectID,
			EnvID:       req.EnvID,
			Name:        req.Name,
			Description: stringOrEmpty(req.Description),
			PolicyID:    req.PolicyID,
			Status:      controlmodel.AgentStatusActive,
			Metadata:    req.Metadata,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "system",
			UpdatedBy:   "system",
		}
		if req.MonthlyBudget != nil {
			budget, err := money.ParseAmount(*req.MonthlyBudget)
			if err != nil {
				return Outcome{Kind: "Agent"}, status.Error(codes.InvalidArgument, "invalid monthly_budget format")
			}
			agent.MonthlyBudget = &budget
		}
		if err := app.CreateAgent(ctx, agent); err != nil {
			return Outcome{Kind: "Agent"}, err
		}
		return Outcome{Kind: "Agent", Data: MapAgentToAPI(agent), Status: 201}, nil
	}
}

func updateAgent(app store.AppStore) InvokeFn {
	type request struct {
		Description   *string        `json:"description,omitempty"`
		PolicyID      *string        `json:"policy_id,omitempty"`
		MonthlyBudget *string        `json:"monthly_budget,omitempty"`
		Metadata      map[string]any `json:"metadata,omitempty"`
	}

	return func(ctx context.Context, body []byte, _ url.Values, path map[string]string) (Outcome, error) {
		id := path["id"]
		agent, err := app.GetAgentByID(ctx, id)
		if err != nil {
			return Outcome{Kind: "Agent"}, err
		}
		if agent == nil {
			return Outcome{Kind: "Agent"}, status.Errorf(codes.NotFound, "agent %q not found", id)
		}

		var req request
		if err := json.Unmarshal(body, &req); err != nil {
			return Outcome{Kind: "Agent"}, status.Error(codes.InvalidArgument, "invalid request body")
		}

		update := &controlmodel.AgentUpdate{
			Description: req.Description,
			PolicyID:    req.PolicyID,
			Metadata:    req.Metadata,
		}
		if req.MonthlyBudget != nil {
			budget, err := money.ParseAmount(*req.MonthlyBudget)
			if err != nil {
				return Outcome{Kind: "Agent"}, status.Error(codes.InvalidArgument, "invalid monthly_budget format")
			}
			update.MonthlyBudget = &budget
		}
		if err := app.UpdateAgent(ctx, id, update); err != nil {
			return Outcome{Kind: "Agent"}, err
		}
		updated, err := app.GetAgentByID(ctx, id)
		if err != nil {
			return Outcome{Kind: "Agent"}, err
		}
		return Outcome{Kind: "Agent", Data: MapAgentToAPI(updated)}, nil
	}
}

func listPolicies(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, query url.Values, _ map[string]string) (Outcome, error) {
		envID := query.Get("env_id")
		if envID == "" {
			return Outcome{}, status.Error(codes.InvalidArgument, "env_id query parameter required")
		}
		limit, cursor := pageFromQuery(query)
		result, err := app.ListPolicies(ctx, envID, store.Page{Limit: limit, Cursor: cursor})
		if err != nil {
			return Outcome{}, err
		}
		data := make([]*Policy, 0, len(result.Items))
		for _, policy := range result.Items {
			data = append(data, MapPolicyToAPI(policy))
		}
		return Outcome{Kind: "PolicyList", Data: data, Page: pageContract(limit, result.NextCursor)}, nil
	}
}

func getPolicy(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		policy, err := app.GetPolicy(ctx, path["id"])
		if err != nil {
			return Outcome{Kind: "Policy"}, err
		}
		if policy == nil {
			return Outcome{Kind: "Policy"}, status.Errorf(codes.NotFound, "policy %q not found", path["id"])
		}
		return Outcome{Kind: "Policy", Data: MapPolicyToAPI(policy)}, nil
	}
}

func createPolicy(app store.AppStore) InvokeFn {
	type request struct {
		ProjectID         string         `json:"project_id"`
		EnvID             string         `json:"env_id"`
		Name              string         `json:"name"`
		Description       *string        `json:"description,omitempty"`
		AllowedTypes      []string       `json:"allowed_types,omitempty"`
		AllowedConnectors []string       `json:"allowed_connectors,omitempty"`
		AllowedMethods    []string       `json:"allowed_methods,omitempty"`
		Config            map[string]any `json:"config,omitempty"`
	}

	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		var req request
		if err := json.Unmarshal(body, &req); err != nil {
			return Outcome{Kind: "Policy"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		if req.Name == "" || req.ProjectID == "" || req.EnvID == "" {
			return Outcome{Kind: "Policy"}, status.Error(codes.InvalidArgument, "name, project_id, and env_id required")
		}
		now := timeNowMS()
		policy := &controlmodel.PolicyRecord{
			ID:                generateID("pol"),
			ProjectID:         req.ProjectID,
			EnvID:             req.EnvID,
			Name:              req.Name,
			Description:       stringOrEmpty(req.Description),
			AllowedTypes:      req.AllowedTypes,
			AllowedConnectors: req.AllowedConnectors,
			AllowedMethods:    req.AllowedMethods,
			TraceInput:        true,
			TraceOutput:       true,
			RetentionDays:     30,
			Mode:              "enforce",
			Status:            "active",
			Config:            req.Config,
			CreatedAt:         now,
			UpdatedAt:         now,
			CreatedBy:         "system",
			UpdatedBy:         "system",
		}
		if err := app.CreatePolicy(ctx, policy); err != nil {
			return Outcome{Kind: "Policy"}, err
		}
		return Outcome{Kind: "Policy", Data: MapPolicyToAPI(policy), Status: 201}, nil
	}
}

func listRuns(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, query url.Values, _ map[string]string) (Outcome, error) {
		filter := &runtimemodel.RunFilter{
			ProjectID: query.Get("project_id"),
			EnvID:     query.Get("env_id"),
			AgentID:   query.Get("agent_id"),
			Status:    query.Get("status"),
		}
		limit, cursor := pageFromQuery(query)
		result, err := app.ListRuns(ctx, filter, store.Page{Limit: limit, Cursor: cursor})
		if err != nil {
			return Outcome{Kind: "RunList"}, err
		}
		data := make([]*Run, 0, len(result.Items))
		for _, run := range result.Items {
			data = append(data, MapRunToAPI(run))
		}
		return Outcome{Kind: "RunList", Data: data, Page: pageContract(limit, result.NextCursor)}, nil
	}
}

func getRun(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, path map[string]string) (Outcome, error) {
		run, err := app.GetRunByID(ctx, path["id"])
		if err != nil {
			return Outcome{Kind: "Run"}, err
		}
		if run == nil {
			return Outcome{Kind: "Run"}, status.Errorf(codes.NotFound, "run %q not found", path["id"])
		}
		return Outcome{Kind: "Run", Data: MapRunToAPI(run)}, nil
	}
}

func getRunSpans(spans store.SpanStore) InvokeFn {
	return func(ctx context.Context, _ []byte, query url.Values, path map[string]string) (Outcome, error) {
		limit, cursor := pageFromQuery(query)
		result, err := spans.QuerySpans(ctx, &runtimemodel.SpanFilter{RunID: path["id"]}, store.Page{Limit: limit, Cursor: cursor})
		if err != nil {
			return Outcome{Kind: "SpanList"}, err
		}
		data := make([]*Span, 0, len(result.Items))
		for _, span := range result.Items {
			data = append(data, MapSpanRowToAPI(span))
		}
		return Outcome{Kind: "SpanList", Data: data, Page: pageContract(limit, result.NextCursor)}, nil
	}
}

func listSpans(spans store.SpanStore) InvokeFn {
	return func(ctx context.Context, _ []byte, query url.Values, _ map[string]string) (Outcome, error) {
		limit, cursor := pageFromQuery(query)
		filter := &runtimemodel.SpanFilter{
			RunID:    query.Get("run_id"),
			HasError: boolQuery(query, "has_error"),
		}
		result, err := spans.QuerySpans(ctx, filter, store.Page{Limit: limit, Cursor: cursor})
		if err != nil {
			return Outcome{Kind: "SpanList"}, err
		}
		data := make([]*Span, 0, len(result.Items))
		for _, span := range result.Items {
			data = append(data, MapSpanRowToAPI(span))
		}
		return Outcome{Kind: "SpanList", Data: data, Page: pageContract(limit, result.NextCursor)}, nil
	}
}

func getCostSummary(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, query url.Values, _ map[string]string) (Outcome, error) {
		report, err := app.GetSpendReport(ctx, &runtimemodel.SpendFilter{
			AgentID:   query.Get("agent_id"),
			Connector: query.Get("connector"),
			Model:     query.Get("model"),
		})
		if err != nil {
			return Outcome{Kind: "SpendReport"}, err
		}

		result := &SpendReport{
			Total:         contract.Money{Amount: report.Total.String(), Currency: defaultCurrency},
			ByAgent:       stringMap(report.ByAgent),
			ByConnector:   stringMap(report.ByConnector),
			ByModel:       stringMap(report.ByModel),
			PeriodStart:   isoFromMS(report.PeriodStart),
			PeriodStartMS: report.PeriodStart,
			PeriodEnd:     isoFromMS(report.PeriodEnd),
			PeriodEndMS:   report.PeriodEnd,
		}
		return Outcome{Kind: "SpendReport", Data: result}, nil
	}
}

func getPriceBook(app store.AppStore) InvokeFn {
	return func(ctx context.Context, _ []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		book, err := app.GetPriceBook(ctx)
		if err != nil {
			return Outcome{Kind: "PriceBook"}, err
		}
		if book == nil {
			return Outcome{Kind: "PriceBook"}, status.Error(codes.NotFound, "price book not found")
		}
		appBook := mappers.ModelPriceBookToApp(book)
		result := &PriceBook{
			Version: appBook.Version,
			Entries: make([]PriceModel, len(appBook.Entries)),
		}
		for i, entry := range appBook.Entries {
			result.Entries[i] = PriceModel{
				Provider:             entry.Provider,
				Match:                entry.Match,
				Source:               entry.Source,
				Currency:             entry.Currency,
				InputPerMillion:      entry.InputPerMillion,
				OutputPerMillion:     entry.OutputPerMillion,
				CacheReadPerMillion:  entry.CacheReadPerMillion,
				CacheWritePerMillion: entry.CacheWritePerMillion,
			}
		}
		return Outcome{Kind: "PriceBook", Data: result}, nil
	}
}

func updatePriceBook(app store.AppStore) InvokeFn {
	return func(ctx context.Context, body []byte, _ url.Values, _ map[string]string) (Outcome, error) {
		var req PriceBook
		if err := json.Unmarshal(body, &req); err != nil {
			return Outcome{Kind: "PriceBook"}, status.Error(codes.InvalidArgument, "invalid request body")
		}
		appBook := &mappers.AppPriceBook{
			Version: req.Version,
			Entries: make([]mappers.AppPriceModel, len(req.Entries)),
		}
		for i, entry := range req.Entries {
			appBook.Entries[i] = mappers.AppPriceModel{
				Provider:             entry.Provider,
				Match:                entry.Match,
				Source:               entry.Source,
				Currency:             entry.Currency,
				InputPerMillion:      entry.InputPerMillion,
				OutputPerMillion:     entry.OutputPerMillion,
				CacheReadPerMillion:  entry.CacheReadPerMillion,
				CacheWritePerMillion: entry.CacheWritePerMillion,
			}
		}
		if err := app.SavePriceBook(ctx, mappers.AppPriceBookToModel(appBook)); err != nil {
			return Outcome{Kind: "PriceBook"}, err
		}
		return Outcome{Kind: "PriceBook", Data: req}, nil
	}
}

func unimplementedRoute(code string) InvokeFn {
	return func(context.Context, []byte, url.Values, map[string]string) (Outcome, error) {
		return Outcome{Kind: "Error"}, status.Error(codes.Unimplemented, code)
	}
}

func pageFromQuery(query url.Values) (int, string) {
	limit := 20
	if raw := query.Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 500 {
		limit = 500
	}
	return limit, query.Get("cursor")
}

func pageContract(limit int, nextCursor string) *contract.Page {
	var cursor *string
	if nextCursor != "" {
		cursor = &nextCursor
	}
	return &contract.Page{Limit: limit, NextCursor: cursor}
}

func boolQuery(query url.Values, key string) *bool {
	raw := query.Get(key)
	if raw == "" {
		return nil
	}
	value := raw == "true" || raw == "1"
	return &value
}

func stringMap(in map[string]money.Amount) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v.String()
	}
	return out
}

func isoFromMS(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
}

func timeNowMS() int64 {
	return time.Now().UnixMilli()
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func generateID(prefix string) string {
	return ids.New(prefix)
}
