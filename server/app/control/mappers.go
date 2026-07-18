package control

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/pkg/ids"
	"github.com/kave-io/kave/core/pkg/money"
	commonv1 "github.com/kave-io/kave/proto/gen/kave/common/v1"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

// ── Amount ────────────────────────────────────────────────────────────────────

func amountToProto(a money.Amount) *commonv1.Amount {
	return &commonv1.Amount{Decimal: a.String()}
}

func amountFromProto(p *commonv1.Amount) money.Amount {
	if p == nil {
		return 0
	}
	a, _ := money.ParseAmount(p.Decimal)
	return a
}

func ptrAmountToProto(a *money.Amount) *commonv1.Amount {
	if a == nil {
		return nil
	}
	return amountToProto(*a)
}

func ptrAmountFromProto(p *commonv1.Amount) *money.Amount {
	if p == nil {
		return nil
	}
	a := amountFromProto(p)
	return &a
}

func structToMap(s *structpb.Struct) map[string]any {
	if s == nil {
		return nil
	}
	return s.AsMap()
}

func mapToStruct(m map[string]any) *structpb.Struct {
	if len(m) == 0 {
		return nil
	}
	s, _ := structpb.NewStruct(m)
	return s
}

// ── Organization ──────────────────────────────────────────────────────────────

func orgToProto(o *control.Organization) *controlv1.Organization {
	if o == nil {
		return nil
	}
	return &controlv1.Organization{
		Id:          o.ID,
		Name:        o.Name,
		Slug:        o.Slug,
		Plan:        planToProto(o.Plan),
		CreatedAtMs: o.CreatedAt,
		UpdatedAtMs: o.UpdatedAt,
	}
}

func planToProto(plan string) controlv1.PlanTier {
	switch plan {
	case "team":
		return controlv1.PlanTier_PLAN_TIER_TEAM
	case "enterprise":
		return controlv1.PlanTier_PLAN_TIER_ENTERPRISE
	default:
		return controlv1.PlanTier_PLAN_TIER_FREE
	}
}

// ── Project ───────────────────────────────────────────────────────────────────

func projectToProto(p *control.Project) *controlv1.Project {
	if p == nil {
		return nil
	}
	return &controlv1.Project{
		Id:          p.ID,
		OrgId:       p.OrgID,
		Name:        p.Name,
		Slug:        p.Slug,
		Description: p.Description,
		CreatedAtMs: p.CreatedAt,
		UpdatedAtMs: p.UpdatedAt,
	}
}

// ── Environment ───────────────────────────────────────────────────────────────

func envToProto(e *control.Environment) *controlv1.Environment {
	if e == nil {
		return nil
	}
	return &controlv1.Environment{
		Id:          e.ID,
		ProjectId:   e.ProjectID,
		Name:        e.Name,
		Slug:        e.Slug,
		Type:        envTypeToProto(e.Type),
		CreatedAtMs: e.CreatedAt,
		UpdatedAtMs: e.UpdatedAt,
	}
}

func envTypeToProto(t string) controlv1.EnvironmentType {
	switch t {
	case "staging":
		return controlv1.EnvironmentType_ENVIRONMENT_TYPE_STAGING
	case "prod":
		return controlv1.EnvironmentType_ENVIRONMENT_TYPE_PROD
	case "custom":
		return controlv1.EnvironmentType_ENVIRONMENT_TYPE_CUSTOM
	default:
		return controlv1.EnvironmentType_ENVIRONMENT_TYPE_DEV
	}
}

func envTypeFromProto(t controlv1.EnvironmentType) string {
	switch t {
	case controlv1.EnvironmentType_ENVIRONMENT_TYPE_STAGING:
		return "staging"
	case controlv1.EnvironmentType_ENVIRONMENT_TYPE_PROD:
		return "prod"
	case controlv1.EnvironmentType_ENVIRONMENT_TYPE_CUSTOM:
		return "custom"
	default:
		return "dev"
	}
}

// ── Agent ─────────────────────────────────────────────────────────────────────

func agentToProto(a *control.Agent) *controlv1.Agent {
	if a == nil {
		return nil
	}
	return &controlv1.Agent{
		Id:            a.ID,
		ProjectId:     a.ProjectID,
		EnvId:         a.EnvID,
		Name:          a.Name,
		Description:   a.Description,
		PolicyId:      a.PolicyID,
		MonthlyBudget: ptrAmountToProto(a.MonthlyBudget),
		Status:        agentStatusToProto(a.Status),
		Metadata:      mapToStruct(a.Metadata),
		CreatedBy:     a.CreatedBy,
		UpdatedBy:     a.UpdatedBy,
		DeletedAtMs:   a.DeletedAt,
		CreatedAtMs:   a.CreatedAt,
		UpdatedAtMs:   a.UpdatedAt,
	}
}

func agentStatusToProto(s string) controlv1.AgentStatus {
	switch s {
	case "disabled":
		return controlv1.AgentStatus_AGENT_STATUS_DISABLED
	default:
		return controlv1.AgentStatus_AGENT_STATUS_ACTIVE
	}
}

func agentStatusFromProto(s controlv1.AgentStatus) string {
	switch s {
	case controlv1.AgentStatus_AGENT_STATUS_DISABLED:
		return "disabled"
	default:
		return "active"
	}
}

// ── Policy ────────────────────────────────────────────────────────────────────

func policyToProto(p *control.PolicyRecord) *controlv1.PolicyRecord {
	if p == nil {
		return nil
	}
	var budgetCap *commonv1.Amount
	if p.BudgetCap != 0 {
		budgetCap = amountToProto(p.BudgetCap)
	}
	return &controlv1.PolicyRecord{
		Id:                p.ID,
		ProjectId:         p.ProjectID,
		EnvId:             p.EnvID,
		Name:              p.Name,
		Description:       p.Description,
		AllowedTypes:      p.AllowedTypes,
		AllowedConnectors: p.AllowedConnectors,
		AllowedMethods:    p.AllowedMethods,
		BudgetCap:         budgetCap,
		BudgetPeriod:      budgetPeriodToProto(p.BudgetPeriod),
		BudgetBehavior:    budgetBehaviorToProto(p.BudgetBehavior),
		TraceInput:        p.TraceInput,
		TraceOutput:       p.TraceOutput,
		RetentionDays:     int32(p.RetentionDays),
		Config:            mapToStruct(p.Config),
		Version:           int32(p.Version),
		Mode:              policyModeToProto(p.Mode),
		Status:            policyStatusToProto(p.Status),
		CreatedBy:         p.CreatedBy,
		UpdatedBy:         p.UpdatedBy,
		CreatedAtMs:       p.CreatedAt,
		UpdatedAtMs:       p.UpdatedAt,
	}
}

func policyModeToProto(m string) controlv1.PolicyMode {
	switch m {
	case "shadow":
		return controlv1.PolicyMode_POLICY_MODE_SHADOW
	default:
		return controlv1.PolicyMode_POLICY_MODE_ENFORCE
	}
}

func policyModeFromProto(m controlv1.PolicyMode) string {
	switch m {
	case controlv1.PolicyMode_POLICY_MODE_SHADOW:
		return "shadow"
	default:
		return "enforce"
	}
}

func policyStatusToProto(s string) controlv1.PolicyStatus {
	switch s {
	case "archived":
		return controlv1.PolicyStatus_POLICY_STATUS_ARCHIVED
	default:
		return controlv1.PolicyStatus_POLICY_STATUS_ACTIVE
	}
}

func policyStatusFromProto(s controlv1.PolicyStatus) string {
	switch s {
	case controlv1.PolicyStatus_POLICY_STATUS_ARCHIVED:
		return "archived"
	default:
		return "active"
	}
}

func budgetPeriodToProto(p string) controlv1.BudgetPeriod {
	switch p {
	case "daily":
		return controlv1.BudgetPeriod_BUDGET_PERIOD_DAILY
	case "monthly":
		return controlv1.BudgetPeriod_BUDGET_PERIOD_MONTHLY
	default:
		return controlv1.BudgetPeriod_BUDGET_PERIOD_RUN
	}
}

func budgetPeriodFromProto(p controlv1.BudgetPeriod) string {
	switch p {
	case controlv1.BudgetPeriod_BUDGET_PERIOD_DAILY:
		return "daily"
	case controlv1.BudgetPeriod_BUDGET_PERIOD_MONTHLY:
		return "monthly"
	default:
		return "run"
	}
}

func budgetBehaviorToProto(b string) controlv1.BudgetBehavior {
	switch b {
	case "warn":
		return controlv1.BudgetBehavior_BUDGET_BEHAVIOR_WARN
	default:
		return controlv1.BudgetBehavior_BUDGET_BEHAVIOR_BLOCK
	}
}

func budgetBehaviorFromProto(b controlv1.BudgetBehavior) string {
	switch b {
	case controlv1.BudgetBehavior_BUDGET_BEHAVIOR_WARN:
		return "warn"
	default:
		return "block"
	}
}

// ── Token ─────────────────────────────────────────────────────────────────────

func tokenToProto(t *control.AgentToken) *controlv1.AgentToken {
	if t == nil {
		return nil
	}
	proto := &controlv1.AgentToken{
		Id:           t.ID,
		AgentId:      t.AgentID,
		ProjectId:    t.ProjectID,
		Name:         t.Name,
		Description:  t.Description,
		TokenPrefix:  t.TokenPrefix,
		Hash:         t.Hash,
		IssuedFor:    t.IssuedFor,
		IssuedBy:     t.IssuedBy,
		Connectors:   t.Connectors,
		Methods:      t.Methods,
		BudgetCap:    ptrAmountToProto(t.BudgetCap),
		Scopes:       t.Scopes,
		NotBeforeMs:  t.NotBefore,
		LastUsedAtMs: t.LastUsedAt,
		RevokedAtMs:  t.RevokedAt,
		RevokedBy:    t.RevokedBy,
		RevokeReason: t.RevokeReason,
		CreatedAtMs:  t.CreatedAt,
	}
	if t.ExpiresAt != 0 {
		proto.ExpiresAtMs = &t.ExpiresAt
	}
	return proto
}

// ── Credential ────────────────────────────────────────────────────────────────

func credentialSourceToProto(s string) controlv1.CredentialSource {
	switch control.CanonicalCredentialSource(s) {
	case control.CredentialSourceEnv:
		return controlv1.CredentialSource_CREDENTIAL_SOURCE_ENV
	case control.CredentialSourceEncrypted:
		return controlv1.CredentialSource_CREDENTIAL_SOURCE_ENCRYPTED
	case control.CredentialSourceVaultRef:
		return controlv1.CredentialSource_CREDENTIAL_SOURCE_VAULT_REF
	case control.CredentialSourceOAuth:
		return controlv1.CredentialSource_CREDENTIAL_SOURCE_OAUTH
	case control.CredentialSourceSTS:
		return controlv1.CredentialSource_CREDENTIAL_SOURCE_STS
	case control.CredentialSourcePassthrough:
		return controlv1.CredentialSource_CREDENTIAL_SOURCE_PASSTHROUGH
	default:
		return controlv1.CredentialSource_CREDENTIAL_SOURCE_UNSPECIFIED
	}
}

func credentialStatusToProto(s string) controlv1.CredentialStatus {
	switch s {
	case "active":
		return controlv1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE
	case "revoked":
		return controlv1.CredentialStatus_CREDENTIAL_STATUS_REVOKED
	case "expired":
		return controlv1.CredentialStatus_CREDENTIAL_STATUS_EXPIRED
	default:
		return controlv1.CredentialStatus_CREDENTIAL_STATUS_UNSPECIFIED
	}
}

func credentialToProto(c *control.ConnectorCredential) *controlv1.ConnectorCredential {
	if c == nil {
		return nil
	}
	return &controlv1.ConnectorCredential{
		Id:            c.ID,
		ProjectId:     c.ProjectID,
		EnvId:         c.EnvID,
		ConnectorType: c.ConnectorType,
		AccountId:     c.AccountID,
		Label:         c.Label,
		Description:   c.Description,
		SourceType:    credentialSourceToProto(credentialSourceName(c)),
		KeyHash:       c.KeyHash,
		WrappingKeyId: c.WrappingKeyID,
		SecretRef:     c.SecretRef,
		SecretVersion: c.SecretVersion,
		Status:        credentialStatusToProto(c.Status),
		Version:       int32(c.Version),
		ExpiresAtMs:   c.ExpiresAt,
		RotatedAtMs:   c.RotatedAt,
		RotatedBy:     c.RotatedBy,
		LastUsedAtMs:  c.LastUsedAt,
		CreatedBy:     c.CreatedBy,
		CreatedAtMs:   c.CreatedAt,
		UpdatedAtMs:   c.UpdatedAt,
		RevokedAtMs:   c.RevokedAt,
		RevokedBy:     c.RevokedBy,
		RevokeReason:  c.RevokeReason,
	}
}

func credentialSourceName(c *control.ConnectorCredential) string {
	if c == nil {
		return ""
	}
	if c.SourceType != "" {
		return c.SourceType
	}
	return string(c.Source)
}

// ── Budget ───────────────────────────────────────────────────────────────────

func budgetToProto(b *control.Budget) *controlv1.Budget {
	if b == nil {
		return nil
	}
	proto := &controlv1.Budget{
		Id:          b.ID,
		AgentId:     b.AgentID,
		HardCap:     amountToProto(b.HardCap),
		Period:      budgetPeriodToProto(b.Period),
		CreatedAtMs: b.CreatedAt,
		UpdatedAtMs: b.UpdatedAt,
	}
	if b.SoftCap != 0 {
		proto.SoftCap = amountToProto(b.SoftCap)
	}
	return proto
}

func budgetFromProto(p *controlv1.Budget) *control.Budget {
	if p == nil {
		return nil
	}
	b := &control.Budget{
		ID:        p.Id,
		AgentID:   p.AgentId,
		HardCap:   amountFromProto(p.HardCap),
		Period:    budgetPeriodFromProto(p.Period),
		CreatedAt: p.CreatedAtMs,
		UpdatedAt: p.UpdatedAtMs,
	}
	if p.SoftCap != nil {
		b.SoftCap = amountFromProto(p.SoftCap)
	}
	return b
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func newID(prefix string) string { return ids.New(prefix) }

func nowMS() int64 {
	return time.Now().UnixMilli()
}

// generateToken produces a raw token, its 8-char prefix, and its SHA256 hex hash.
func generateToken() (raw, prefix, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("generate token: %w", err)
	}
	raw = "kave_" + hex.EncodeToString(b)
	prefix = raw[:8]
	sum := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(sum[:])
	return
}

// ── RBAC ───────────────────────────────────────────────────────────────────

func roleToProto(r *control.Role) *controlv1.Role {
	if r == nil {
		return nil
	}
	return &controlv1.Role{
		Id:          r.ID,
		Name:        r.Name,
		Permissions: r.Permissions,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func bindingToProto(b *control.Binding) *controlv1.Binding {
	if b == nil {
		return nil
	}
	return &controlv1.Binding{
		Id:        b.ID,
		RoleId:    b.RoleID,
		Subject:   b.Subject,
		Scope:     b.Scope,
		CreatedAt: b.CreatedAt,
	}
}
