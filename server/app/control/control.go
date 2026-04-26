package control

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/kave-io/kave/core/bus"
	"github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/store"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	"github.com/kave-io/kave/server/internal/daemon"
	serverauth "github.com/kave-io/kave/server/ops/auth"
	"go.yaml.in/yaml/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Server implements controlv1.ControlPlaneServiceServer and coordinates all control-plane services.
type Server struct {
	controlv1.UnimplementedControlPlaneServiceServer

	appStore store.AppStore
	bus      *bus.Bus

	// Child services
	authService   *AuthServiceImpl
	rbacService   *RBACServiceImpl
	daemonService *DaemonServiceImpl
}

// New creates a new ControlPlaneAPI server.
func New(appStore store.AppStore, b *bus.Bus) *Server {
	return &Server{appStore: appStore, bus: b}
}

// RegisterWithChildren registers all control-plane services with gRPC.
// Call this instead of Register if you want all services including auth/rbac/daemon.
func (s *Server) RegisterWithChildren(srv *grpc.Server, daemonState *daemon.State, tokens *serverauth.TokenManager) {
	// Main ControlPlaneService
	controlv1.RegisterControlPlaneServiceServer(srv, s)

	// Auth service
	if tokens != nil {
		s.authService = NewAuthService(s.appStore, tokens)
		controlv1.RegisterAuthServiceServer(srv, s.authService)
	}

	// RBAC service
	s.rbacService = NewRBACService(s.appStore)
	controlv1.RegisterRBACServiceServer(srv, s.rbacService)

	// Daemon service
	if daemonState != nil {
		s.daemonService = NewDaemonService(daemonState)
		controlv1.RegisterDaemonServiceServer(srv, s.daemonService)
	}
}

// Register registers the ControlPlaneService server with gRPC.
func (s *Server) Register(srv *grpc.Server) {
	controlv1.RegisterControlPlaneServiceServer(srv, s)
}

// ── Organization Operations ────────────────────────────────────────────────

func (s *Server) CreateOrganization(ctx context.Context, req *controlv1.CreateOrganizationRequest) (*controlv1.Organization, error) {
	now := nowMS()
	org := &control.Organization{
		ID:        newID("org"),
		Name:      req.Name,
		Slug:      req.Slug,
		Plan:      "free",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.appStore.CreateOrg(ctx, org); err != nil {
		return nil, err
	}
	return orgToProto(org), nil
}

func (s *Server) GetOrganization(ctx context.Context, req *controlv1.GetOrganizationRequest) (*controlv1.Organization, error) {
	org, err := s.appStore.GetOrg(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, status.Errorf(codes.NotFound, "organization %q not found", req.Id)
	}
	return orgToProto(org), nil
}

func (s *Server) ListOrganizations(ctx context.Context, req *controlv1.ListOrganizationsRequest) (*controlv1.ListOrganizationsResponse, error) {
	result, err := s.appStore.ListOrgs(ctx, store.Page{Limit: int(req.Limit), Cursor: req.Cursor})
	if err != nil {
		return nil, err
	}
	resp := &controlv1.ListOrganizationsResponse{}
	for _, o := range result.Items {
		resp.Organizations = append(resp.Organizations, orgToProto(o))
	}
	resp.NextCursor = result.NextCursor
	return resp, nil
}

// ── Project Operations ─────────────────────────────────────────────────────

func (s *Server) CreateProject(ctx context.Context, req *controlv1.CreateProjectRequest) (*controlv1.Project, error) {
	now := nowMS()
	proj := &control.Project{
		ID:        newID("prj"),
		OrgID:     req.OrgId,
		Name:      req.Name,
		Slug:      req.Slug,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.appStore.CreateProject(ctx, proj); err != nil {
		return nil, err
	}
	return projectToProto(proj), nil
}

func (s *Server) GetProject(ctx context.Context, req *controlv1.GetProjectRequest) (*controlv1.Project, error) {
	proj, err := s.appStore.GetProject(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if proj == nil {
		return nil, status.Errorf(codes.NotFound, "project %q not found", req.Id)
	}
	return projectToProto(proj), nil
}

func (s *Server) ListProjects(ctx context.Context, req *controlv1.ListProjectsRequest) (*controlv1.ListProjectsResponse, error) {
	page := store.Page{Limit: int(req.Limit), Cursor: req.Cursor}
	result, err := s.appStore.ListProjects(ctx, req.OrgId, page)
	if err != nil {
		return nil, err
	}
	resp := &controlv1.ListProjectsResponse{}
	for _, p := range result.Items {
		resp.Projects = append(resp.Projects, projectToProto(p))
	}
	resp.NextCursor = result.NextCursor
	return resp, nil
}

// ── Environment Operations ─────────────────────────────────────────────────

func (s *Server) CreateEnvironment(ctx context.Context, req *controlv1.CreateEnvironmentRequest) (*controlv1.Environment, error) {
	now := nowMS()
	env := &control.Environment{
		ID:        newID("env"),
		ProjectID: req.ProjectId,
		Name:      req.Name,
		Slug:      req.Slug,
		Type:      envTypeFromProto(req.Type),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.appStore.CreateEnvironment(ctx, env); err != nil {
		return nil, err
	}
	return envToProto(env), nil
}

func (s *Server) GetEnvironment(ctx context.Context, req *controlv1.GetEnvironmentRequest) (*controlv1.Environment, error) {
	env, err := s.appStore.GetEnvironment(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if env == nil {
		return nil, status.Errorf(codes.NotFound, "environment %q not found", req.Id)
	}
	return envToProto(env), nil
}

func (s *Server) ListEnvironments(ctx context.Context, req *controlv1.ListEnvironmentsRequest) (*controlv1.ListEnvironmentsResponse, error) {
	page := store.Page{Limit: int(req.Limit), Cursor: req.Cursor}
	result, err := s.appStore.ListEnvironments(ctx, req.ProjectId, page)
	if err != nil {
		return nil, err
	}
	resp := &controlv1.ListEnvironmentsResponse{}
	for _, e := range result.Items {
		resp.Environments = append(resp.Environments, envToProto(e))
	}
	resp.NextCursor = result.NextCursor
	return resp, nil
}

// ── Agent Operations ───────────────────────────────────────────────────────

func (s *Server) CreateAgent(ctx context.Context, req *controlv1.CreateAgentRequest) (*controlv1.Agent, error) {
	now := nowMS()
	agent := &control.Agent{
		ID:          newID("agn"),
		EnvID:       req.EnvId,
		Name:        req.Name,
		Description: req.Description,
		Status:      control.AgentStatusActive,
		Metadata:    map[string]any{},
		CreatedBy:   "system",
		UpdatedBy:   "system",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if req.PolicyId != nil {
		agent.PolicyID = req.PolicyId
	}
	if err := s.appStore.CreateAgent(ctx, agent); err != nil {
		return nil, err
	}
	s.publish("agent.created", agent)
	return agentToProto(agent), nil
}

func (s *Server) GetAgent(ctx context.Context, req *controlv1.GetAgentRequest) (*controlv1.Agent, error) {
	agent, err := s.appStore.GetAgentByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return nil, status.Errorf(codes.NotFound, "agent %q not found", req.Id)
	}
	return agentToProto(agent), nil
}

func (s *Server) ListAgents(ctx context.Context, req *controlv1.ListAgentsRequest) (*controlv1.ListAgentsResponse, error) {
	page := store.Page{Limit: int(req.Limit), Cursor: req.Cursor}
	result, err := s.appStore.ListAgents(ctx, req.EnvId, page)
	if err != nil {
		return nil, err
	}
	resp := &controlv1.ListAgentsResponse{}
	for _, a := range result.Items {
		resp.Agents = append(resp.Agents, agentToProto(a))
	}
	resp.NextCursor = result.NextCursor
	return resp, nil
}

func (s *Server) UpdateAgent(ctx context.Context, req *controlv1.UpdateAgentRequest) (*controlv1.Agent, error) {
	if req.Update == nil {
		return s.GetAgent(ctx, &controlv1.GetAgentRequest{Id: req.Id})
	}
	u := req.Update
	update := &control.AgentUpdate{
		UpdatedBy: u.UpdatedBy, // *string, matches AgentUpdate.UpdatedBy
	}
	if u.Description != nil {
		update.Description = u.Description
	}
	if u.PolicyId != nil {
		update.PolicyID = u.PolicyId
	}
	if u.MonthlyBudget != nil {
		a := amountFromProto(u.MonthlyBudget)
		update.MonthlyBudget = &a
	}
	if u.Status != nil {
		sv := agentStatusFromProto(*u.Status)
		update.Status = &sv
	}
	if u.Metadata != nil {
		update.Metadata = structToMap(u.Metadata)
	}
	if err := s.appStore.UpdateAgent(ctx, req.Id, update); err != nil {
		return nil, err
	}
	agent, err := s.GetAgent(ctx, &controlv1.GetAgentRequest{Id: req.Id})
	if err == nil {
		s.publish("agent.updated", protoToAgent(agent))
	}
	return agent, err
}

func (s *Server) DeleteAgent(ctx context.Context, req *controlv1.DeleteAgentRequest) (*emptypb.Empty, error) {
	agent, err := s.GetAgent(ctx, &controlv1.GetAgentRequest{Id: req.Id})
	if err != nil {
		return nil, err
	}
	if err := s.appStore.DeleteAgent(ctx, req.Id, "system"); err != nil {
		return nil, err
	}
	s.publish("agent.deleted", protoToAgent(agent))
	return &emptypb.Empty{}, nil
}

func (s *Server) RestoreAgent(ctx context.Context, req *controlv1.RestoreAgentRequest) (*controlv1.Agent, error) {
	if err := s.appStore.RestoreAgent(ctx, req.Id, "system"); err != nil {
		return nil, err
	}
	agent, err := s.GetAgent(ctx, &controlv1.GetAgentRequest{Id: req.Id})
	if err == nil {
		s.publish("agent.restored", protoToAgent(agent))
	}
	return agent, err
}

// ── Policy Operations ──────────────────────────────────────────────────────
// TODO: policy test, apply/diff, credential test, and connector operations
// are deferred to later plans.

func (s *Server) CreatePolicy(ctx context.Context, req *controlv1.CreatePolicyRequest) (*controlv1.PolicyRecord, error) {
	now := nowMS()
	pol := &control.PolicyRecord{
		ID:                newID("pol"),
		EnvID:             req.EnvId,
		Name:              req.Name,
		Description:       req.Description,
		AllowedTypes:      []string{"*"},
		AllowedConnectors: []string{"*"},
		AllowedMethods:    []string{"*"},
		TraceInput:        true,
		TraceOutput:       true,
		RetentionDays:     30,
		BudgetPeriod:      "run",
		BudgetBehavior:    "block",
		Mode:              policyModeFromProto(req.Mode),
		Status:            "active",
		CreatedBy:         "system",
		UpdatedBy:         "system",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.appStore.CreatePolicy(ctx, pol); err != nil {
		return nil, err
	}
	s.publish("policy.created", pol)
	return policyToProto(pol), nil
}

func (s *Server) GetPolicy(ctx context.Context, req *controlv1.GetPolicyRequest) (*controlv1.PolicyRecord, error) {
	pol, err := s.appStore.GetPolicy(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if pol == nil {
		return nil, status.Errorf(codes.NotFound, "policy %q not found", req.Id)
	}
	return policyToProto(pol), nil
}

func (s *Server) ListPolicies(ctx context.Context, req *controlv1.ListPoliciesRequest) (*controlv1.ListPoliciesResponse, error) {
	page := store.Page{Limit: int(req.Limit), Cursor: req.Cursor}
	result, err := s.appStore.ListPolicies(ctx, req.EnvId, page)
	if err != nil {
		return nil, err
	}
	resp := &controlv1.ListPoliciesResponse{}
	for _, p := range result.Items {
		resp.Policies = append(resp.Policies, policyToProto(p))
	}
	resp.NextCursor = result.NextCursor
	return resp, nil
}

func (s *Server) UpdatePolicy(ctx context.Context, req *controlv1.UpdatePolicyRequest) (*controlv1.PolicyRecord, error) {
	if req.Update == nil {
		return s.GetPolicy(ctx, &controlv1.GetPolicyRequest{Id: req.Id})
	}
	u := req.Update
	update := &control.PolicyUpdate{
		Description:       u.Description,
		AllowedTypes:      u.AllowedTypes,
		AllowedConnectors: u.AllowedConnectors,
		AllowedMethods:    u.AllowedMethods,
		ClearBudgetCap:    u.ClearBudgetCap,
		UpdatedBy:         u.UpdatedBy,
	}
	if u.BudgetCap != nil {
		a := amountFromProto(u.BudgetCap)
		update.BudgetCap = &a
	}
	if u.BudgetPeriod != nil {
		s := budgetPeriodFromProto(*u.BudgetPeriod)
		update.BudgetPeriod = &s
	}
	if u.BudgetBehavior != nil {
		s := budgetBehaviorFromProto(*u.BudgetBehavior)
		update.BudgetBehavior = &s
	}
	if u.TraceInput != nil {
		update.TraceInput = u.TraceInput
	}
	if u.TraceOutput != nil {
		update.TraceOutput = u.TraceOutput
	}
	if u.RetentionDays != nil {
		v := int(*u.RetentionDays)
		update.RetentionDays = &v
	}
	if u.Config != nil {
		update.Config = structToMap(u.Config)
	}
	if u.Mode != nil {
		m := policyModeFromProto(*u.Mode)
		update.Mode = &m
	}
	if u.Status != nil {
		st := policyStatusFromProto(*u.Status)
		update.Status = &st
	}
	if err := s.appStore.UpdatePolicy(ctx, req.Id, update); err != nil {
		return nil, err
	}
	pol, err := s.GetPolicy(ctx, &controlv1.GetPolicyRequest{Id: req.Id})
	if err == nil {
		s.publish("policy.updated", protoToPolicy(pol))
	}
	return pol, err
}

func (s *Server) DeletePolicy(ctx context.Context, req *controlv1.DeletePolicyRequest) (*emptypb.Empty, error) {
	pol, err := s.GetPolicy(ctx, &controlv1.GetPolicyRequest{Id: req.Id})
	if err != nil {
		return nil, err
	}
	if err := s.appStore.DeletePolicy(ctx, req.Id); err != nil {
		return nil, err
	}
	s.publish("policy.deleted", protoToPolicy(pol))
	return &emptypb.Empty{}, nil
}

func (s *Server) ExportPolicy(ctx context.Context, req *controlv1.ExportPolicyRequest) (*controlv1.PolicyYAML, error) {
	pol, err := s.appStore.GetPolicy(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if pol == nil {
		return nil, status.Errorf(codes.NotFound, "policy %q not found", req.Id)
	}
	doc := policyYAMLFromModel(pol)
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return &controlv1.PolicyYAML{Yaml: string(raw)}, nil
}

func (s *Server) ValidatePolicy(ctx context.Context, req *controlv1.ValidatePolicyRequest) (*controlv1.ValidatePolicyResponse, error) {
	var doc policyYAMLDocument
	dec := yaml.NewDecoder(strings.NewReader(req.Yaml))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return &controlv1.ValidatePolicyResponse{
			Ok:     false,
			Issues: []string{err.Error()},
		}, nil
	}
	issues := doc.validate()
	return &controlv1.ValidatePolicyResponse{
		Ok:     len(issues) == 0,
		Issues: issues,
	}, nil
}

// ── Token Operations ───────────────────────────────────────────────────────

func (s *Server) CreateToken(ctx context.Context, req *controlv1.CreateTokenRequest) (*controlv1.CreateTokenResponse, error) {
	raw, prefix, hash, err := generateToken()
	if err != nil {
		return nil, err
	}
	now := nowMS()
	tok := &control.AgentToken{
		ID:          newID("tok"),
		AgentID:     req.AgentId,
		Name:        req.Name,
		TokenPrefix: prefix,
		Hash:        hash,
		IssuedFor:   "human",
		IssuedBy:    "system",
		NotBefore:   now,
		CreatedAt:   now,
	}
	if err := s.appStore.InsertAgentToken(ctx, tok); err != nil {
		return nil, err
	}
	return &controlv1.CreateTokenResponse{
		Token:    tokenToProto(tok),
		RawToken: raw,
	}, nil
}

func (s *Server) GetToken(ctx context.Context, req *controlv1.GetTokenRequest) (*controlv1.AgentToken, error) {
	tok, err := s.appStore.GetToken(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if tok == nil {
		return nil, status.Errorf(codes.NotFound, "token %q not found", req.Id)
	}
	return tokenToProto(tok), nil
}

func (s *Server) ListTokens(ctx context.Context, req *controlv1.ListTokensRequest) (*controlv1.ListTokensResponse, error) {
	page := store.Page{Limit: int(req.Limit), Cursor: req.Cursor}
	result, err := s.appStore.ListTokens(ctx, req.AgentId, page)
	if err != nil {
		return nil, err
	}
	resp := &controlv1.ListTokensResponse{}
	for _, t := range result.Items {
		resp.Tokens = append(resp.Tokens, tokenToProto(t))
	}
	resp.NextCursor = result.NextCursor
	return resp, nil
}

func (s *Server) RevokeToken(ctx context.Context, req *controlv1.RevokeTokenRequest) (*emptypb.Empty, error) {
	if err := s.appStore.RevokeToken(ctx, req.Id, "system", req.Reason); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ── Credential Operations ──────────────────────────────────────────────────

func (s *Server) CreateCredential(ctx context.Context, req *controlv1.CreateCredentialRequest) (*controlv1.ConnectorCredential, error) {
	now := nowMS()
	cred := &control.ConnectorCredential{
		ID:            newID("crd"),
		EnvID:         req.EnvId,
		ConnectorType: req.ConnectorType,
		Label:         req.Label,
		SourceType:    "encrypted",
		Status:        "active",
		CreatedBy:     "system",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.appStore.StoreCredential(ctx, cred); err != nil {
		return nil, err
	}
	s.publish("credential.created", cred)
	return credentialToProto(cred), nil
}

func (s *Server) GetCredential(ctx context.Context, req *controlv1.GetCredentialRequest) (*controlv1.ConnectorCredential, error) {
	cred, err := s.appStore.GetCredential(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return nil, status.Errorf(codes.NotFound, "credential %q not found", req.Id)
	}
	return credentialToProto(cred), nil
}

func (s *Server) ListCredentials(ctx context.Context, req *controlv1.ListCredentialsRequest) (*controlv1.ListCredentialsResponse, error) {
	var envID string
	if req.Filter != nil {
		envID = req.Filter.EnvId
	}
	page := store.Page{Limit: int(req.Limit), Cursor: req.Cursor}
	result, err := s.appStore.ListCredentials(ctx, envID, page)
	if err != nil {
		return nil, err
	}
	resp := &controlv1.ListCredentialsResponse{}
	for _, c := range result.Items {
		resp.Credentials = append(resp.Credentials, credentialToProto(c))
	}
	resp.NextCursor = result.NextCursor
	return resp, nil
}

func (s *Server) UpdateCredential(ctx context.Context, req *controlv1.UpdateCredentialRequest) (*controlv1.ConnectorCredential, error) {
	cred, err := s.appStore.GetCredential(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return nil, status.Errorf(codes.NotFound, "credential %q not found", req.Id)
	}
	if req.Update != nil {
		if req.Update.Label != nil {
			cred.Label = *req.Update.Label
		}
		if req.Update.Description != nil {
			cred.Description = *req.Update.Description
		}
		if req.Update.AccountId != nil {
			cred.AccountID = *req.Update.AccountId
		}
	}
	cred.UpdatedAt = nowMS()
	if err := s.appStore.StoreCredential(ctx, cred); err != nil {
		return nil, err
	}
	s.publish("credential.updated", cred)
	return credentialToProto(cred), nil
}

func (s *Server) RotateCredential(ctx context.Context, req *controlv1.RotateCredentialRequest) (*controlv1.ConnectorCredential, error) {
	if err := s.appStore.RotateCredential(ctx, req.Id, req.NewEncryptedBlob, "", "system"); err != nil {
		return nil, err
	}
	cred, err := s.GetCredential(ctx, &controlv1.GetCredentialRequest{Id: req.Id})
	if err == nil {
		s.publish("credential.rotated", protoToCredential(cred))
	}
	return cred, err
}

func (s *Server) RevokeCredential(ctx context.Context, req *controlv1.RevokeCredentialRequest) (*emptypb.Empty, error) {
	if err := s.appStore.RevokeCredential(ctx, req.Id, "system", req.Reason); err != nil {
		return nil, err
	}
	s.publish("credential.revoked", &control.ConnectorCredential{ID: req.Id})
	return &emptypb.Empty{}, nil
}

func (s *Server) DeleteCredential(ctx context.Context, req *controlv1.DeleteCredentialRequest) (*emptypb.Empty, error) {
	cred, err := s.GetCredential(ctx, &controlv1.GetCredentialRequest{Id: req.Id})
	if err != nil {
		return nil, err
	}
	if err := s.appStore.DeleteCredential(ctx, req.Id); err != nil {
		return nil, err
	}
	s.publish("credential.deleted", protoToCredential(cred))
	return &emptypb.Empty{}, nil
}

func (s *Server) CreateBudget(ctx context.Context, req *controlv1.CreateBudgetRequest) (*controlv1.Budget, error) {
	now := nowMS()
	budget := &control.Budget{
		ID:        newID("bgt"),
		AgentID:   req.AgentId,
		HardCap:   amountFromProto(req.HardCap),
		Period:    budgetPeriodFromProto(req.Period),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if req.SoftCap != nil {
		budget.SoftCap = amountFromProto(req.SoftCap)
	}
	if err := s.appStore.CreateBudget(ctx, budget); err != nil {
		return nil, err
	}
	s.publish("budget.updated", budget)
	return budgetToProto(budget), nil
}

func (s *Server) GetBudget(ctx context.Context, req *controlv1.GetBudgetRequest) (*controlv1.Budget, error) {
	budget, err := s.appStore.GetBudget(ctx, req.AgentId)
	if err != nil {
		return nil, err
	}
	if budget == nil {
		return nil, status.Errorf(codes.NotFound, "budget for agent %q not found", req.AgentId)
	}
	return budgetToProto(budget), nil
}

func (s *Server) DeleteBudget(ctx context.Context, req *controlv1.DeleteBudgetRequest) (*emptypb.Empty, error) {
	budget, err := s.GetBudget(ctx, &controlv1.GetBudgetRequest{AgentId: req.AgentId})
	if err != nil {
		return nil, err
	}
	if err := s.appStore.DeleteBudget(ctx, req.AgentId); err != nil {
		return nil, err
	}
	s.publish("budget.deleted", budgetFromProto(budget))
	return &emptypb.Empty{}, nil
}

func (s *Server) publish(kind string, v any) {
	if s == nil || s.bus == nil || v == nil {
		return
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	ev := bus.Event{
		Kind:    kind,
		At:      nowMS(),
		Payload: raw,
	}
	switch x := v.(type) {
	case *control.Agent:
		ev.ProjectID = x.ProjectID
		ev.EnvID = x.EnvID
		ev.AgentID = x.ID
	case *control.PolicyRecord:
		ev.ProjectID = x.ProjectID
		ev.EnvID = x.EnvID
	case *control.ConnectorCredential:
		ev.ProjectID = x.ProjectID
		ev.EnvID = x.EnvID
	case *control.Budget:
		ev.AgentID = x.AgentID
	case *controlv1.Agent:
		ev.ProjectID = x.ProjectId
		ev.EnvID = x.EnvId
		ev.AgentID = x.Id
	case *controlv1.PolicyRecord:
		ev.ProjectID = x.ProjectId
		ev.EnvID = x.EnvId
	case *controlv1.ConnectorCredential:
		ev.ProjectID = x.ProjectId
		ev.EnvID = x.EnvId
	case *controlv1.Budget:
		ev.AgentID = x.AgentId
	}
	s.bus.Publish(ev)
}

func protoToAgent(a *controlv1.Agent) *control.Agent {
	if a == nil {
		return nil
	}
	model := &control.Agent{
		ID:          a.Id,
		ProjectID:   a.ProjectId,
		EnvID:       a.EnvId,
		Name:        a.Name,
		Description: a.Description,
		PolicyID:    a.PolicyId,
		Status:      agentStatusFromProto(a.Status),
		Metadata:    structToMap(a.Metadata),
		CreatedBy:   a.CreatedBy,
		UpdatedBy:   a.UpdatedBy,
		DeletedAt:   a.DeletedAtMs,
		CreatedAt:   a.CreatedAtMs,
		UpdatedAt:   a.UpdatedAtMs,
	}
	if a.MonthlyBudget != nil {
		amount := amountFromProto(a.MonthlyBudget)
		model.MonthlyBudget = &amount
	}
	return model
}

func protoToPolicy(p *controlv1.PolicyRecord) *control.PolicyRecord {
	if p == nil {
		return nil
	}
	model := &control.PolicyRecord{
		ID:                p.Id,
		ProjectID:         p.ProjectId,
		EnvID:             p.EnvId,
		Name:              p.Name,
		Description:       p.Description,
		AllowedTypes:      append([]string(nil), p.AllowedTypes...),
		AllowedConnectors: append([]string(nil), p.AllowedConnectors...),
		AllowedMethods:    append([]string(nil), p.AllowedMethods...),
		BudgetPeriod:      budgetPeriodFromProto(p.BudgetPeriod),
		BudgetBehavior:    budgetBehaviorFromProto(p.BudgetBehavior),
		TraceInput:        p.TraceInput,
		TraceOutput:       p.TraceOutput,
		RetentionDays:     int(p.RetentionDays),
		Config:            structToMap(p.Config),
		Version:           int(p.Version),
		Mode:              policyModeFromProto(p.Mode),
		Status:            policyStatusFromProto(p.Status),
		CreatedBy:         p.CreatedBy,
		UpdatedBy:         p.UpdatedBy,
		CreatedAt:         p.CreatedAtMs,
		UpdatedAt:         p.UpdatedAtMs,
	}
	if p.BudgetCap != nil {
		amount := amountFromProto(p.BudgetCap)
		model.BudgetCap = amount
	}
	return model
}

func protoToCredential(c *controlv1.ConnectorCredential) *control.ConnectorCredential {
	if c == nil {
		return nil
	}
	model := &control.ConnectorCredential{
		ID:            c.Id,
		ProjectID:     c.ProjectId,
		EnvID:         c.EnvId,
		ConnectorType: c.ConnectorType,
		AccountID:     c.AccountId,
		Label:         c.Label,
		Description:   c.Description,
		SourceType:    credentialSourceFromProto(c.SourceType),
		KeyHash:       c.KeyHash,
		WrappingKeyID: c.WrappingKeyId,
		SecretRef:     c.SecretRef,
		SecretVersion: c.SecretVersion,
		Status:        credentialStatusFromProto(c.Status),
		Version:       int(c.Version),
		ExpiresAt:     c.ExpiresAtMs,
		RotatedAt:     c.RotatedAtMs,
		RotatedBy:     c.RotatedBy,
		LastUsedAt:    c.LastUsedAtMs,
		CreatedBy:     c.CreatedBy,
		CreatedAt:     c.CreatedAtMs,
		UpdatedAt:     c.UpdatedAtMs,
		RevokedAt:     c.RevokedAtMs,
		RevokedBy:     c.RevokedBy,
		RevokeReason:  c.RevokeReason,
	}
	return model
}

func credentialSourceFromProto(s controlv1.CredentialSource) string {
	switch s {
	case controlv1.CredentialSource_CREDENTIAL_SOURCE_ENCRYPTED:
		return "encrypted"
	case controlv1.CredentialSource_CREDENTIAL_SOURCE_VAULT_REF:
		return "vault_ref"
	case controlv1.CredentialSource_CREDENTIAL_SOURCE_OAUTH:
		return "oauth"
	case controlv1.CredentialSource_CREDENTIAL_SOURCE_STS:
		return "sts"
	case controlv1.CredentialSource_CREDENTIAL_SOURCE_PASSTHROUGH:
		return "passthrough"
	default:
		return ""
	}
}

func credentialStatusFromProto(s controlv1.CredentialStatus) string {
	switch s {
	case controlv1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE:
		return "active"
	case controlv1.CredentialStatus_CREDENTIAL_STATUS_REVOKED:
		return "revoked"
	case controlv1.CredentialStatus_CREDENTIAL_STATUS_EXPIRED:
		return "expired"
	default:
		return ""
	}
}

type policyYAMLDocument struct {
	ID                string         `yaml:"id"`
	ProjectID         string         `yaml:"project_id"`
	EnvID             string         `yaml:"env_id"`
	Name              string         `yaml:"name"`
	Description       string         `yaml:"description"`
	AllowedTypes      []string       `yaml:"allowed_types"`
	AllowedConnectors []string       `yaml:"allowed_connectors"`
	AllowedMethods    []string       `yaml:"allowed_methods"`
	BudgetCap         *string        `yaml:"budget_cap"`
	BudgetPeriod      string         `yaml:"budget_period"`
	BudgetBehavior    string         `yaml:"budget_behavior"`
	TraceInput        bool           `yaml:"trace_input"`
	TraceOutput       bool           `yaml:"trace_output"`
	RetentionDays     int            `yaml:"retention_days"`
	Config            map[string]any `yaml:"config"`
	Mode              string         `yaml:"mode"`
	Status            string         `yaml:"status"`
}

func policyYAMLFromModel(p *control.PolicyRecord) policyYAMLDocument {
	doc := policyYAMLDocument{
		ID:                p.ID,
		ProjectID:         p.ProjectID,
		EnvID:             p.EnvID,
		Name:              p.Name,
		Description:       p.Description,
		AllowedTypes:      p.AllowedTypes,
		AllowedConnectors: p.AllowedConnectors,
		AllowedMethods:    p.AllowedMethods,
		BudgetPeriod:      p.BudgetPeriod,
		BudgetBehavior:    p.BudgetBehavior,
		TraceInput:        p.TraceInput,
		TraceOutput:       p.TraceOutput,
		RetentionDays:     p.RetentionDays,
		Config:            p.Config,
		Mode:              p.Mode,
		Status:            p.Status,
	}
	if p.BudgetCap != 0 {
		s := p.BudgetCap.String()
		doc.BudgetCap = &s
	}
	return doc
}

func (d policyYAMLDocument) validate() []string {
	var issues []string
	if d.ID == "" {
		issues = append(issues, "id is required")
	}
	if d.ProjectID == "" {
		issues = append(issues, "project_id is required")
	}
	if d.EnvID == "" {
		issues = append(issues, "env_id is required")
	}
	if d.Name == "" {
		issues = append(issues, "name is required")
	}
	return issues
}
