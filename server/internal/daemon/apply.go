package daemon

import (
	"context"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	controlmodel "github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/config"
)

type ApplyPlan struct {
	Creates []ResourceOp `json:"creates"`
	Updates []ResourceOp `json:"updates"`
	Deletes []ResourceOp `json:"deletes"`
}

type ResourceOp struct {
	Kind   string        `json:"kind"`
	Name   string        `json:"name"`
	Before any           `json:"before,omitempty"`
	After  any           `json:"after,omitempty"`
	Source config.Source `json:"source"`
}

type ApplyReport struct {
	Created int       `json:"created"`
	Updated int       `json:"updated"`
	Deleted int       `json:"deleted"`
	Plan    ApplyPlan `json:"plan"`
}

func (s *State) BuildPlan(ctx context.Context) (ApplyPlan, error) {
	return s.buildPlanForConfig(ctx, s.currentConfig())
}

func (s *State) buildPlanForConfig(ctx context.Context, cfg *config.Config) (ApplyPlan, error) {
	plan := ApplyPlan{}

	projectID, project := desiredProject(cfg)
	if project != nil {
		existing, err := s.app.GetProject(ctx, projectID)
		if err != nil {
			return plan, err
		}
		if existing == nil {
			plan.Creates = append(plan.Creates, ResourceOp{Kind: "project", Name: project.Name, After: project, Source: config.SourceProject})
		} else if !reflect.DeepEqual(normalizeProject(existing), normalizeProject(project)) {
			plan.Updates = append(plan.Updates, ResourceOp{Kind: "project", Name: project.Name, Before: existing, After: project, Source: config.SourceProject})
		}
	}

	envs := desiredEnvironments(cfg, projectID)
	existingEnvs, err := listAllEnvironments(ctx, s.app, projectID)
	if err != nil {
		return plan, err
	}
	existingEnvBySlug := map[string]*controlmodel.Environment{}
	for _, env := range existingEnvs {
		existingEnvBySlug[env.Slug] = env
	}
	for _, env := range envs {
		existing := existingEnvBySlug[env.Slug]
		if existing == nil {
			plan.Creates = append(plan.Creates, ResourceOp{Kind: "environment", Name: env.Name, After: env, Source: config.SourceProject})
			continue
		}
		if !reflect.DeepEqual(normalizeEnvironment(existing), normalizeEnvironment(env)) {
			plan.Updates = append(plan.Updates, ResourceOp{Kind: "environment", Name: env.Name, Before: existing, After: env, Source: config.SourceProject})
		}
	}

	desiredPolicies := desiredPolicies(cfg, projectID, defaultEnvSlug(cfg))
	existingPolicies, err := listAllPolicies(ctx, s.app, defaultEnvSlug(cfg))
	if err != nil {
		return plan, err
	}
	existingPolicyByName := make(map[string]*controlmodel.PolicyRecord, len(existingPolicies))
	for _, pol := range existingPolicies {
		existingPolicyByName[pol.Name] = pol
	}
	for _, desired := range desiredPolicies {
		existing := existingPolicyByName[desired.Name]
		if existing == nil {
			plan.Creates = append(plan.Creates, ResourceOp{Kind: "policy", Name: desired.Name, After: desired, Source: s.sourceForResource("policy", desired.Name)})
			continue
		}
		if !reflect.DeepEqual(normalizePolicy(existing), normalizePolicy(desired)) {
			plan.Updates = append(plan.Updates, ResourceOp{Kind: "policy", Name: desired.Name, Before: existing, After: desired, Source: s.sourceForResource("policy", desired.Name)})
		}
	}

	desiredCreds := desiredCredentials(cfg, projectID, defaultEnvSlug(cfg))
	existingCreds, err := listAllCredentials(ctx, s.app, defaultEnvSlug(cfg))
	if err != nil {
		return plan, err
	}
	existingCredByName := make(map[string]*controlmodel.ConnectorCredential, len(existingCreds))
	for _, cred := range existingCreds {
		existingCredByName[cred.ID] = cred
		existingCredByName[cred.Label] = cred
		existingCredByName[cred.ConnectorType+":"+cred.Label] = cred
	}
	for _, desired := range desiredCreds {
		existing := existingCredByName[desired.ID]
		if existing == nil {
			existing = existingCredByName[desired.Label]
		}
		if existing == nil {
			existing = existingCredByName[desired.ConnectorType+":"+desired.Label]
		}
		if existing == nil {
			plan.Creates = append(plan.Creates, ResourceOp{Kind: "credential", Name: desired.ID, After: desired, Source: s.sourceForResource("credential", desired.ID)})
			continue
		}
		if !reflect.DeepEqual(normalizeCredential(existing), normalizeCredential(desired)) {
			plan.Updates = append(plan.Updates, ResourceOp{Kind: "credential", Name: desired.ID, Before: existing, After: desired, Source: s.sourceForResource("credential", desired.ID)})
		}
	}

	plan.Deletes = append(plan.Deletes, s.prunableDeletes(cfg, projectID, defaultEnvSlug(cfg))...)
	sortResourceOps(plan.Creates)
	sortResourceOps(plan.Updates)
	sortResourceOps(plan.Deletes)
	return plan, nil
}

func (s *State) Apply(ctx context.Context, plan ApplyPlan, prune bool) (ApplyReport, error) {
	report := ApplyReport{Plan: plan}

	for _, kind := range []string{"project", "environment", "policy", "credential", "agent", "connector"} {
		for _, op := range filterOps(plan.Creates, kind) {
			if err := s.applyCreate(ctx, op); err != nil {
				return report, err
			}
			report.Created++
		}
	}
	for _, kind := range []string{"project", "environment", "policy", "credential", "agent", "connector"} {
		for _, op := range filterOps(plan.Updates, kind) {
			if err := s.applyUpdate(ctx, op); err != nil {
				return report, err
			}
			report.Updated++
		}
	}
	if prune {
		for _, kind := range []string{"agent", "credential", "policy", "environment", "project", "connector"} {
			for _, op := range filterOps(plan.Deletes, kind) {
				if err := s.applyDelete(ctx, op); err != nil {
					return report, err
				}
				report.Deleted++
			}
		}
	}
	return report, nil
}

func (s *State) prunableDeletes(cfg *config.Config, projectID, envID string) []ResourceOp {
	_ = cfg
	s.resourceMu.RLock()
	defer s.resourceMu.RUnlock()

	var deletes []ResourceOp
	for key, source := range s.resourceSource {
		if source != config.SourceProject && source != config.SourceUser {
			continue
		}
		kind, name, ok := splitResourceKey(key)
		if !ok {
			continue
		}
		if s.resourceExistsInConfig(kind, name, cfg) {
			continue
		}
		deletes = append(deletes, ResourceOp{Kind: kind, Name: name, Source: source})
	}
	return deletes
}

func (s *State) applyCreate(ctx context.Context, op ResourceOp) error {
	switch op.Kind {
	case "project":
		if p, ok := op.After.(*controlmodel.Project); ok {
			if err := s.app.CreateProject(ctx, p); err != nil {
				return err
			}
			s.recordResourceSource("project", op.Name, op.Source)
			return nil
		}
	case "environment":
		if e, ok := op.After.(*controlmodel.Environment); ok {
			if err := s.app.CreateEnvironment(ctx, e); err != nil {
				return err
			}
			s.recordResourceSource("environment", op.Name, op.Source)
			return nil
		}
	case "policy":
		if p, ok := op.After.(*controlmodel.PolicyRecord); ok {
			if err := s.app.CreatePolicy(ctx, p); err != nil {
				return err
			}
			s.recordResourceSource("policy", op.Name, op.Source)
			return nil
		}
	case "credential":
		if c, ok := op.After.(*controlmodel.ConnectorCredential); ok {
			if err := s.app.StoreCredential(ctx, c); err != nil {
				return err
			}
			s.recordResourceSource("credential", op.Name, op.Source)
			return nil
		}
	case "agent":
		if a, ok := op.After.(*controlmodel.Agent); ok {
			if err := s.app.CreateAgent(ctx, a); err != nil {
				return err
			}
			s.recordResourceSource("agent", op.Name, op.Source)
			return nil
		}
	}
	return nil
}

func (s *State) applyUpdate(ctx context.Context, op ResourceOp) error {
	switch op.Kind {
	case "policy":
		if desired, ok := op.After.(*controlmodel.PolicyRecord); ok {
			if err := updatePolicyRecord(ctx, s.app, desired); err != nil {
				return err
			}
			s.recordResourceSource("policy", op.Name, op.Source)
			return nil
		}
	case "credential":
		if desired, ok := op.After.(*controlmodel.ConnectorCredential); ok {
			if err := s.app.StoreCredential(ctx, desired); err != nil {
				return err
			}
			s.recordResourceSource("credential", op.Name, op.Source)
			return nil
		}
	case "agent":
		if desired, ok := op.After.(*controlmodel.Agent); ok {
			if err := updateAgentRecord(ctx, s.app, desired); err != nil {
				return err
			}
			s.recordResourceSource("agent", op.Name, op.Source)
			return nil
		}
	}
	return nil
}

func (s *State) applyDelete(ctx context.Context, op ResourceOp) error {
	switch op.Kind {
	case "policy":
		return deletePolicyByName(ctx, s.app, op.Name, defaultEnvSlug(s.currentConfig()))
	case "credential":
		return deleteCredentialByName(ctx, s.app, op.Name, defaultEnvSlug(s.currentConfig()))
	case "agent":
		return deleteAgentByName(ctx, s.app, op.Name, defaultEnvSlug(s.currentConfig()))
	}
	return nil
}

func defaultEnvSlug(cfg *config.Config) string {
	if cfg != nil && cfg.Project != nil && cfg.Project.DefaultEnv != "" {
		return cfg.Project.DefaultEnv
	}
	return "default"
}

func desiredProject(cfg *config.Config) (string, *controlmodel.Project) {
	if cfg == nil || cfg.Project == nil {
		return "default", nil
	}
	id := cfg.Project.Slug
	if id == "" {
		id = cfg.Project.Name
	}
	if id == "" {
		id = "default"
	}
	now := time.Now().UnixMilli()
	return id, &controlmodel.Project{
		ID:          id,
		OrgID:       "default",
		Name:        cfg.Project.Name,
		Slug:        cfg.Project.Slug,
		Description: cfg.Project.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func desiredEnvironments(cfg *config.Config, projectID string) []*controlmodel.Environment {
	if cfg == nil || cfg.Project == nil || len(cfg.Project.Envs) == 0 {
		return []*controlmodel.Environment{{
			ID:        defaultEnvSlug(cfg),
			ProjectID: projectID,
			Name:      defaultEnvSlug(cfg),
			Slug:      defaultEnvSlug(cfg),
			Type:      controlmodel.EnvTypeDev,
			CreatedAt: time.Now().UnixMilli(),
			UpdatedAt: time.Now().UnixMilli(),
		}}
	}
	now := time.Now().UnixMilli()
	out := make([]*controlmodel.Environment, 0, len(cfg.Project.Envs))
	for _, env := range cfg.Project.Envs {
		slug := env.Slug
		if slug == "" {
			slug = env.Name
		}
		out = append(out, &controlmodel.Environment{
			ID:        slug,
			ProjectID: projectID,
			Name:      env.Name,
			Slug:      slug,
			Type:      env.Type,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	return out
}

func desiredPolicies(cfg *config.Config, projectID, envID string) []*controlmodel.PolicyRecord {
	now := time.Now().UnixMilli()
	var out []*controlmodel.PolicyRecord
	for _, pol := range cfg.Policies {
		name := pol.Name
		if name == "" {
			continue
		}
		var budgetCap money.Amount
		if pol.Cost != nil {
			if capMap, ok := pol.Cost["budgetCap"].(map[string]any); ok {
				budgetCap = moneyFromMap(capMap)
			}
		}
		out = append(out, &controlmodel.PolicyRecord{
			ID:                name,
			ProjectID:         projectID,
			EnvID:             envID,
			Name:              name,
			Description:       pol.Description,
			AllowedTypes:      stringSliceFromAny(pol.Auth, "allowedTypes"),
			AllowedConnectors: stringSliceFromAny(pol.Auth, "allowedConnectors"),
			AllowedMethods:    stringSliceFromAny(pol.Auth, "allowedMethods"),
			BudgetCap:         budgetCap,
			BudgetPeriod:      stringFromNested(pol.Cost, "budgetPeriod"),
			BudgetBehavior:    stringFromNested(pol.Cost, "budgetBehavior"),
			TraceInput:        boolFromNested(pol.Trace, "input"),
			TraceOutput:       boolFromNested(pol.Trace, "output"),
			RetentionDays:     intFromNested(pol.Trace, "retentionDays"),
			Config:            pol.Validation,
			Mode:              pol.Mode,
			Status:            string(controlmodel.PolicyStatusActive),
			CreatedBy:         "system",
			UpdatedBy:         "system",
			CreatedAt:         now,
			UpdatedAt:         now,
		})
	}
	return out
}

func desiredCredentials(cfg *config.Config, projectID, envID string) []*controlmodel.ConnectorCredential {
	now := time.Now().UnixMilli()
	var out []*controlmodel.ConnectorCredential
	for _, cred := range cfg.Credentials {
		if cred.Name == "" {
			continue
		}
		out = append(out, &controlmodel.ConnectorCredential{
			ID:            cred.Name,
			ProjectID:     projectID,
			EnvID:         envID,
			ConnectorType: cred.Connector,
			Label:         cred.Label,
			SourceType:    credentialSourceType(cred.Source),
			SecretRef:     credentialSecretRef(cred),
			SecretVersion: cred.Ref,
			Status:        controlmodel.CredStatusActive,
			CreatedBy:     "system",
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}
	return out
}

func credentialSourceType(source string) string {
	switch strings.ToLower(source) {
	case "passthrough":
		return controlmodel.CredSourcePassthrough
	case "vault":
		return controlmodel.CredSourceVaultRef
	case "file", "env":
		return controlmodel.CredSourceVaultRef
	default:
		return controlmodel.CredSourceVaultRef
	}
}

func credentialSecretRef(cred config.CredentialConfig) string {
	switch strings.ToLower(cred.Source) {
	case "env":
		return cred.Env
	case "file":
		return cred.File
	case "vault":
		return cred.Ref
	default:
		return cred.Ref
	}
}

func (s *State) sourceForResource(kind, name string) config.Source {
	s.resourceMu.RLock()
	defer s.resourceMu.RUnlock()
	if s.resourceSource == nil {
		return config.SourceProject
	}
	if source, ok := s.resourceSource[resourceKey(kind, name)]; ok {
		return source
	}
	return config.SourceProject
}

func normalizeProject(p *controlmodel.Project) map[string]any {
	if p == nil {
		return nil
	}
	return map[string]any{"id": p.ID, "org_id": p.OrgID, "name": p.Name, "slug": p.Slug, "description": p.Description}
}

func normalizeEnvironment(e *controlmodel.Environment) map[string]any {
	if e == nil {
		return nil
	}
	return map[string]any{"id": e.ID, "project_id": e.ProjectID, "name": e.Name, "slug": e.Slug, "type": e.Type}
}

func normalizePolicy(p *controlmodel.PolicyRecord) map[string]any {
	if p == nil {
		return nil
	}
	return map[string]any{
		"id":                 p.ID,
		"project_id":         p.ProjectID,
		"env_id":             p.EnvID,
		"name":               p.Name,
		"description":        p.Description,
		"allowed_types":      slices.Clone(p.AllowedTypes),
		"allowed_connectors": slices.Clone(p.AllowedConnectors),
		"allowed_methods":    slices.Clone(p.AllowedMethods),
		"budget_cap":         p.BudgetCap,
		"budget_period":      p.BudgetPeriod,
		"budget_behavior":    p.BudgetBehavior,
		"trace_input":        p.TraceInput,
		"trace_output":       p.TraceOutput,
		"retention_days":     p.RetentionDays,
		"config":             p.Config,
		"mode":               p.Mode,
		"status":             p.Status,
	}
}

func normalizeCredential(c *controlmodel.ConnectorCredential) map[string]any {
	if c == nil {
		return nil
	}
	return map[string]any{
		"id":             c.ID,
		"project_id":     c.ProjectID,
		"env_id":         c.EnvID,
		"connector_type": c.ConnectorType,
		"label":          c.Label,
		"description":    c.Description,
		"source_type":    c.SourceType,
		"secret_ref":     c.SecretRef,
		"secret_version": c.SecretVersion,
		"status":         c.Status,
	}
}

func stringSliceFromAny(section map[string]any, key string) []string {
	if section == nil {
		return nil
	}
	raw, ok := section[key]
	if !ok {
		return nil
	}
	values, ok := raw.([]any)
	if !ok {
		if xs, ok := raw.([]string); ok {
			return slices.Clone(xs)
		}
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if s, ok := value.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func stringFromNested(section map[string]any, key string) string {
	if section == nil {
		return ""
	}
	if value, ok := section[key].(string); ok {
		return value
	}
	return ""
}

func boolFromNested(section map[string]any, key string) bool {
	if section == nil {
		return false
	}
	if value, ok := section[key].(bool); ok {
		return value
	}
	return false
}

func intFromNested(section map[string]any, key string) int {
	if section == nil {
		return 0
	}
	switch value := section[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func moneyFromMap(m map[string]any) money.Amount {
	if m == nil {
		return 0
	}
	amount, _ := m["amount"].(string)
	if amount == "" {
		return 0
	}
	parsed, err := money.ParseAmount(amount)
	if err != nil {
		return 0
	}
	return parsed
}

func updatePolicyRecord(ctx context.Context, app store.AppStore, desired *controlmodel.PolicyRecord) error {
	existing, err := app.GetPolicy(ctx, desired.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return app.CreatePolicy(ctx, desired)
	}
	update := &controlmodel.PolicyUpdate{
		Description:       strPtrIfDifferent(existing.Description, desired.Description),
		AllowedTypes:      desired.AllowedTypes,
		AllowedConnectors: desired.AllowedConnectors,
		AllowedMethods:    desired.AllowedMethods,
		BudgetPeriod:      strPtrIfDifferent(existing.BudgetPeriod, desired.BudgetPeriod),
		BudgetBehavior:    strPtrIfDifferent(existing.BudgetBehavior, desired.BudgetBehavior),
		TraceInput:        boolPtrIfDifferent(existing.TraceInput, desired.TraceInput),
		TraceOutput:       boolPtrIfDifferent(existing.TraceOutput, desired.TraceOutput),
		RetentionDays:     intPtrIfDifferent(existing.RetentionDays, desired.RetentionDays),
		Config:            desired.Config,
		Mode:              strPtrIfDifferent(existing.Mode, desired.Mode),
		Status:            strPtrIfDifferent(existing.Status, desired.Status),
	}
	if desired.BudgetCap > 0 {
		cap := desired.BudgetCap
		update.BudgetCap = &cap
	} else {
		update.ClearBudgetCap = true
	}
	return app.UpdatePolicy(ctx, desired.ID, update)
}

func updateAgentRecord(ctx context.Context, app store.AppStore, desired *controlmodel.Agent) error {
	existing, err := app.GetAgentByID(ctx, desired.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return app.CreateAgent(ctx, desired)
	}
	update := &controlmodel.AgentUpdate{
		Description:   strPtrIfDifferent(existing.Description, desired.Description),
		PolicyID:      stringPtrIfDifferent(existing.PolicyID, desired.PolicyID),
		MonthlyBudget: amountPtrIfDifferent(existing.MonthlyBudget, desired.MonthlyBudget),
		Status:        strPtrIfDifferent(existing.Status, desired.Status),
		Metadata:      desired.Metadata,
	}
	return app.UpdateAgent(ctx, desired.ID, update)
}

func deletePolicyByName(ctx context.Context, app store.AppStore, name, envID string) error {
	policies, err := listAllPolicies(ctx, app, envID)
	if err != nil {
		return err
	}
	for _, pol := range policies {
		if pol.Name == name {
			return app.DeletePolicy(ctx, pol.ID)
		}
	}
	return nil
}

func deleteAgentByName(ctx context.Context, app store.AppStore, name, envID string) error {
	agents, err := listAllAgents(ctx, app, envID)
	if err != nil {
		return err
	}
	for _, agent := range agents {
		if agent.Name == name {
			return app.DeleteAgent(ctx, agent.ID, "system")
		}
	}
	return nil
}

func deleteCredentialByName(ctx context.Context, app store.AppStore, name, envID string) error {
	creds, err := listAllCredentials(ctx, app, envID)
	if err != nil {
		return err
	}
	for _, cred := range creds {
		if cred.ID == name {
			return app.DeleteCredential(ctx, cred.ID)
		}
	}
	return nil
}

func listAllEnvironments(ctx context.Context, app store.AppStore, projectID string) ([]*controlmodel.Environment, error) {
	return listAll(ctx, func(page store.Page) (store.PageResult[*controlmodel.Environment], error) {
		return app.ListEnvironments(ctx, projectID, page)
	})
}

func listAllPolicies(ctx context.Context, app store.AppStore, envID string) ([]*controlmodel.PolicyRecord, error) {
	return listAll(ctx, func(page store.Page) (store.PageResult[*controlmodel.PolicyRecord], error) {
		return app.ListPolicies(ctx, envID, page)
	})
}

func listAllCredentials(ctx context.Context, app store.AppStore, envID string) ([]*controlmodel.ConnectorCredential, error) {
	return listAll(ctx, func(page store.Page) (store.PageResult[*controlmodel.ConnectorCredential], error) {
		return app.ListCredentials(ctx, envID, page)
	})
}

func listAllAgents(ctx context.Context, app store.AppStore, envID string) ([]*controlmodel.Agent, error) {
	return listAll(ctx, func(page store.Page) (store.PageResult[*controlmodel.Agent], error) {
		return app.ListAgents(ctx, envID, page)
	})
}

func listAll[T any](ctx context.Context, fn func(page store.Page) (store.PageResult[T], error)) ([]T, error) {
	var out []T
	cursor := ""
	for {
		page := store.Page{Limit: 500, Cursor: cursor}
		result, err := fn(page)
		if err != nil {
			return nil, err
		}
		out = append(out, result.Items...)
		if result.NextCursor == "" {
			return out, nil
		}
		cursor = result.NextCursor
	}
}

func sortResourceOps(ops []ResourceOp) {
	sort.SliceStable(ops, func(i, j int) bool {
		if ops[i].Kind != ops[j].Kind {
			return ops[i].Kind < ops[j].Kind
		}
		return ops[i].Name < ops[j].Name
	})
}

func filterOps(ops []ResourceOp, kind string) []ResourceOp {
	out := make([]ResourceOp, 0, len(ops))
	for _, op := range ops {
		if op.Kind == kind {
			out = append(out, op)
		}
	}
	return out
}

func splitResourceKey(key string) (kind, name string, ok bool) {
	kind, name, ok = strings.Cut(key, ":")
	return kind, name, ok
}

func (s *State) resourceExistsInConfig(kind, name string, cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	switch kind {
	case "project":
		return cfg.Project != nil
	case "environment":
		if cfg.Project == nil {
			return false
		}
		for _, env := range cfg.Project.Envs {
			slug := env.Slug
			if slug == "" {
				slug = env.Name
			}
			if slug == name || env.Name == name {
				return true
			}
		}
		return false
	case "agent":
		for _, item := range cfg.Agents {
			if item.Name == name {
				return true
			}
		}
	case "policy":
		for _, item := range cfg.Policies {
			if item.Name == name {
				return true
			}
		}
	case "credential":
		for _, item := range cfg.Credentials {
			if item.Name == name {
				return true
			}
		}
	}
	return false
}

func (s *State) recordResourceSource(kind, name string, source config.Source) {
	s.resourceMu.Lock()
	defer s.resourceMu.Unlock()
	if s.resourceSource == nil {
		s.resourceSource = map[string]config.Source{}
	}
	s.resourceSource[resourceKey(kind, name)] = source
}

func resourceKey(kind, name string) string {
	return kind + ":" + name
}

func strPtrIfDifferent(existing, desired string) *string {
	if existing == desired {
		return nil
	}
	return &desired
}

func stringPtrIfDifferent(existing, desired *string) *string {
	if existing == nil && desired == nil {
		return nil
	}
	if existing != nil && desired != nil && *existing == *desired {
		return nil
	}
	if desired == nil {
		return nil
	}
	return desired
}

func boolPtrIfDifferent(existing, desired bool) *bool {
	if existing == desired {
		return nil
	}
	return &desired
}

func intPtrIfDifferent(existing, desired int) *int {
	if existing == desired {
		return nil
	}
	return &desired
}

func amountPtrIfDifferent(existing, desired *money.Amount) *money.Amount {
	if existing == nil && desired == nil {
		return nil
	}
	if existing != nil && desired != nil && *existing == *desired {
		return nil
	}
	return desired
}
