package control

import (
	"context"

	"github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/store"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Server implements controlv1.ControlPlaneServiceServer.
type Server struct {
	controlv1.UnimplementedControlPlaneServiceServer

	appStore store.AppStore
}

// New creates a new ControlPlaneAPI server.
func New(appStore store.AppStore) *Server {
	return &Server{appStore: appStore}
}

// Register registers the ControlPlaneService server with gRPC.
func (s *Server) Register(srv *grpc.Server) {
	controlv1.RegisterControlPlaneServiceServer(srv, s)
}

// ── Organization Operations ────────────────────────────────────────────────

func (s *Server) CreateOrganization(ctx context.Context, req *controlv1.CreateOrganizationRequest) (*controlv1.Organization, error) {
	now := nowMS()
	org := &control.Organization{
		ID:        newID(),
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
	// Not in OrgStore interface — return empty list for now.
	return &controlv1.ListOrganizationsResponse{}, nil
}

// ── Project Operations ─────────────────────────────────────────────────────

func (s *Server) CreateProject(ctx context.Context, req *controlv1.CreateProjectRequest) (*controlv1.Project, error) {
	now := nowMS()
	proj := &control.Project{
		ID:        newID(),
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
	page := store.Page{Limit: int(req.Limit)}
	result, err := s.appStore.ListProjects(ctx, req.OrgId, page)
	if err != nil {
		return nil, err
	}
	resp := &controlv1.ListProjectsResponse{}
	for _, p := range result.Items {
		resp.Projects = append(resp.Projects, projectToProto(p))
	}
	return resp, nil
}

// ── Environment Operations ─────────────────────────────────────────────────

func (s *Server) CreateEnvironment(ctx context.Context, req *controlv1.CreateEnvironmentRequest) (*controlv1.Environment, error) {
	now := nowMS()
	env := &control.Environment{
		ID:        newID(),
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
	page := store.Page{Limit: int(req.Limit)}
	result, err := s.appStore.ListEnvironments(ctx, req.ProjectId, page)
	if err != nil {
		return nil, err
	}
	resp := &controlv1.ListEnvironmentsResponse{}
	for _, e := range result.Items {
		resp.Environments = append(resp.Environments, envToProto(e))
	}
	return resp, nil
}

// ── Agent Operations ───────────────────────────────────────────────────────

func (s *Server) CreateAgent(ctx context.Context, req *controlv1.CreateAgentRequest) (*controlv1.Agent, error) {
	now := nowMS()
	agent := &control.Agent{
		ID:          newID(),
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
	page := store.Page{Limit: int(req.Limit)}
	result, err := s.appStore.ListAgents(ctx, req.EnvId, page)
	if err != nil {
		return nil, err
	}
	resp := &controlv1.ListAgentsResponse{}
	for _, a := range result.Items {
		resp.Agents = append(resp.Agents, agentToProto(a))
	}
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
	return s.GetAgent(ctx, &controlv1.GetAgentRequest{Id: req.Id})
}

func (s *Server) DeleteAgent(ctx context.Context, req *controlv1.DeleteAgentRequest) (*emptypb.Empty, error) {
	if err := s.appStore.DeleteAgent(ctx, req.Id, "system"); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ── Policy Operations ──────────────────────────────────────────────────────

func (s *Server) CreatePolicy(ctx context.Context, req *controlv1.CreatePolicyRequest) (*controlv1.PolicyRecord, error) {
	now := nowMS()
	pol := &control.PolicyRecord{
		ID:                newID(),
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
	page := store.Page{Limit: int(req.Limit)}
	result, err := s.appStore.ListPolicies(ctx, req.EnvId, page)
	if err != nil {
		return nil, err
	}
	resp := &controlv1.ListPoliciesResponse{}
	for _, p := range result.Items {
		resp.Policies = append(resp.Policies, policyToProto(p))
	}
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
	return s.GetPolicy(ctx, &controlv1.GetPolicyRequest{Id: req.Id})
}

// ── Token Operations ───────────────────────────────────────────────────────

func (s *Server) CreateToken(ctx context.Context, req *controlv1.CreateTokenRequest) (*controlv1.CreateTokenResponse, error) {
	raw, prefix, hash := generateToken()
	now := nowMS()
	tok := &control.AgentToken{
		ID:          newID(),
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
	page := store.Page{Limit: int(req.Limit)}
	result, err := s.appStore.ListTokens(ctx, req.AgentId, page)
	if err != nil {
		return nil, err
	}
	resp := &controlv1.ListTokensResponse{}
	for _, t := range result.Items {
		resp.Tokens = append(resp.Tokens, tokenToProto(t))
	}
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
		ID:            newID(),
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
	page := store.Page{Limit: int(req.Limit)}
	result, err := s.appStore.ListCredentials(ctx, envID, page)
	if err != nil {
		return nil, err
	}
	resp := &controlv1.ListCredentialsResponse{}
	for _, c := range result.Items {
		resp.Credentials = append(resp.Credentials, credentialToProto(c))
	}
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
	return credentialToProto(cred), nil
}

func (s *Server) RotateCredential(ctx context.Context, req *controlv1.RotateCredentialRequest) (*controlv1.ConnectorCredential, error) {
	if err := s.appStore.RotateCredential(ctx, req.Id, req.NewEncryptedBlob, "", "system"); err != nil {
		return nil, err
	}
	return s.GetCredential(ctx, &controlv1.GetCredentialRequest{Id: req.Id})
}

func (s *Server) RevokeCredential(ctx context.Context, req *controlv1.RevokeCredentialRequest) (*emptypb.Empty, error) {
	if err := s.appStore.RevokeCredential(ctx, req.Id, "system", req.Reason); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

