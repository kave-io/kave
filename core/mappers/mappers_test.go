package mappers

import (
	"testing"

	controlmodel "github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/pkg/timex"
)

// ── Agent ──────────────────────────────────────────────────────────────────

func TestAgentCreateToModel_fields(t *testing.T) {
	ts := int64(1_000_000)
	budget := money.MustParseAmount("25.00")
	in := &AgentCreateInput{
		ID:            "agt_1",
		ProjectID:     "prj_1",
		EnvID:         "env_1",
		Name:          "my-agent",
		Description:   "test agent",
		MonthlyBudget: &budget,
		CreatedBy:     "usr_1",
		CreatedAt:     &ts,
	}
	out := AgentCreateToModel(in)
	if out == nil {
		t.Fatal("expected non-nil")
	}
	if out.ID != "agt_1" || out.Name != "my-agent" {
		t.Errorf("unexpected fields: %+v", out)
	}
	if out.Status != controlmodel.AgentStatusActive {
		t.Errorf("expected active status, got %q", out.Status)
	}
	if out.CreatedAt != ts || out.UpdatedAt != ts {
		t.Errorf("timestamps mismatch: createdAt=%d updatedAt=%d want %d", out.CreatedAt, out.UpdatedAt, ts)
	}
	if out.MonthlyBudget == nil || *out.MonthlyBudget != budget {
		t.Errorf("budget mismatch: %v", out.MonthlyBudget)
	}
}

func TestAgentCreateToModel_nilSafe(t *testing.T) {
	if AgentCreateToModel(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentCreateToModel_defaultsCreatedBy(t *testing.T) {
	out := AgentCreateToModel(&AgentCreateInput{ID: "x", CreatedBy: ""})
	if out.CreatedBy != "system" {
		t.Errorf("expected system, got %q", out.CreatedBy)
	}
}

func TestAgentToView_nilSafe(t *testing.T) {
	if AgentToView(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentToView_budgetFormatted(t *testing.T) {
	budget := money.MustParseAmount("10.50")
	a := &controlmodel.Agent{
		ID:            "agt_2",
		MonthlyBudget: &budget,
		Status:        controlmodel.AgentStatusActive,
	}
	v := AgentToView(a)
	if v.MonthlyBudget == nil {
		t.Fatal("expected budget string in view")
	}
	if *v.MonthlyBudget == "" {
		t.Error("budget string must not be empty")
	}
}

func TestAgentToView_nilBudget(t *testing.T) {
	a := &controlmodel.Agent{ID: "agt_3", Status: controlmodel.AgentStatusActive}
	v := AgentToView(a)
	if v.MonthlyBudget != nil {
		t.Errorf("expected nil budget in view, got %v", v.MonthlyBudget)
	}
}

func TestAgentUpdateToModel_nilSafe(t *testing.T) {
	if AgentUpdateToModel(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentUpdateToModel_fieldsPassThrough(t *testing.T) {
	desc := "new desc"
	status := "inactive"
	in := &AgentUpdateInput{Description: &desc, Status: &status}
	out := AgentUpdateToModel(in)
	if out == nil {
		t.Fatal("expected non-nil")
	}
	if out.Description == nil || *out.Description != "new desc" {
		t.Errorf("description mismatch: %v", out.Description)
	}
	if out.Status == nil || *out.Status != "inactive" {
		t.Errorf("status mismatch: %v", out.Status)
	}
}

// ── Run ────────────────────────────────────────────────────────────────────

func TestRunToRecord_roundTrip(t *testing.T) {
	corrID := "corr-123"
	sesID := "ses_1"
	errMsg := "timeout"
	polID := "pol_1"
	r := &runtime.Run{
		ID:            "run_1",
		ProjectID:     "prj_1",
		EnvID:         "env_1",
		AgentID:       "agt_1",
		PolicyID:      &polID,
		Name:          "my-run",
		Status:        runtime.RunCompleted,
		StartedAt:     timex.MS(1000),
		EndedAt:       timex.MS(2000),
		Spent:         money.MustParseAmount("1.23"),
		Error:         &errMsg,
		TriggerType:   runtime.TriggerAPI,
		CorrelationID: &corrID,
		SessionID:     &sesID,
		Metadata:      map[string]any{"k": "v"},
	}
	cap := money.MustParseAmount("5.00")
	rec := RunToRecord(r, cap)
	if rec == nil {
		t.Fatal("expected non-nil record")
	}
	if rec.ID != "run_1" || rec.Status != "completed" {
		t.Errorf("unexpected record: %+v", rec)
	}
	if rec.BudgetCap != cap {
		t.Errorf("budget cap mismatch: %v vs %v", rec.BudgetCap, cap)
	}
	if rec.ErrorMessage == nil || *rec.ErrorMessage != "timeout" {
		t.Errorf("error message mismatch: %v", rec.ErrorMessage)
	}

	// round-trip back
	back := RecordToRun(rec)
	if back == nil {
		t.Fatal("expected non-nil run")
	}
	if back.Status != runtime.RunCompleted {
		t.Errorf("status mismatch: %v", back.Status)
	}
	if back.TriggerType != runtime.TriggerAPI {
		t.Errorf("trigger type mismatch: %v", back.TriggerType)
	}
	if back.CorrelationID == nil || *back.CorrelationID != "corr-123" {
		t.Errorf("correlation ID mismatch: %v", back.CorrelationID)
	}
}

func TestRunToRecord_nilSafe(t *testing.T) {
	if RunToRecord(nil, 0) != nil {
		t.Fatal("expected nil for nil run")
	}
}

func TestRecordToRun_nilSafe(t *testing.T) {
	if RecordToRun(nil) != nil {
		t.Fatal("expected nil for nil record")
	}
}

func TestRunUpdate_nilSafe(t *testing.T) {
	if RunUpdate(nil) != nil {
		t.Fatal("expected nil for nil run")
	}
}

func TestRunUpdate_fields(t *testing.T) {
	errMsg := "fail"
	r := &runtime.Run{
		Status:  runtime.RunFailed,
		Spent:   money.MustParseAmount("0.05"),
		Error:   &errMsg,
		EndedAt: timex.MS(9999),
	}
	u := RunUpdate(r)
	if u == nil {
		t.Fatal("expected non-nil update")
	}
	if u.Status == nil || *u.Status != "failed" {
		t.Errorf("status mismatch: %v", u.Status)
	}
	if u.Spent == nil || *u.Spent != money.MustParseAmount("0.05") {
		t.Errorf("spent mismatch: %v", u.Spent)
	}
	if u.ErrorMessage == nil || *u.ErrorMessage != "fail" {
		t.Errorf("error message mismatch: %v", u.ErrorMessage)
	}
}

// ── Audit ──────────────────────────────────────────────────────────────────

func TestAuditEntryToModel_fields(t *testing.T) {
	ts := int64(5_000_000)
	ip := "192.168.1.1"
	in := &AuditEntryInput{
		ID:           "aud_1",
		OrgID:        "org_1",
		ActorID:      "usr_1",
		ActorType:    "user",
		Event:        "agent.created",
		ResourceType: "agent",
		ResourceID:   "agt_1",
		Before:       nil,
		After:        map[string]any{"name": "new-agent"},
		IP:           &ip,
		CreatedAt:    &ts,
	}
	out := AuditEntryToModel(in)
	if out == nil {
		t.Fatal("expected non-nil")
	}
	if out.ID != "aud_1" || out.Event != "agent.created" {
		t.Errorf("unexpected output: %+v", out)
	}
	if out.DiffBefore != nil {
		t.Errorf("expected nil DiffBefore for nil Before, got %v", out.DiffBefore)
	}
	if out.DiffAfter == nil {
		t.Error("expected non-nil DiffAfter")
	}
	if out.CreatedAt != ts {
		t.Errorf("timestamp mismatch: %d vs %d", out.CreatedAt, ts)
	}
	if out.IP == nil || *out.IP != "192.168.1.1" {
		t.Errorf("IP mismatch: %v", out.IP)
	}
}

func TestAuditEntryToModel_nilSafe(t *testing.T) {
	if AuditEntryToModel(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
}

// ── Budget ─────────────────────────────────────────────────────────────────

func TestBudgetEntryFromUsage_fields(t *testing.T) {
	actionID := "act_1"
	spanID := "spn_1"
	ts := int64(99)
	in := &BudgetEntryInput{
		ID:        "bge_1",
		ProjectID: "prj_1",
		EnvID:     "env_1",
		AgentID:   "agt_1",
		RunID:     "run_1",
		ActionID:  &actionID,
		SpanID:    &spanID,
		Connector: "openai",
		TokenUsage: &runtime.TokenUsage{
			InputTokens:  100,
			OutputTokens: 50,
			CacheRead:    10,
			Model:        "gpt-4",
		},
		Cost:      money.MustParseAmount("0.03"),
		Currency:  money.USD,
		CreatedAt: &ts,
	}
	out := BudgetEntryFromUsage(in)
	if out == nil {
		t.Fatal("expected non-nil")
	}
	if out.Cost != money.MustParseAmount("0.03") {
		t.Errorf("cost mismatch: %v", out.Cost)
	}
	if out.Model != "gpt-4" {
		t.Errorf("model mismatch: %q", out.Model)
	}
	if out.InputTokens != 100 {
		t.Errorf("input tokens mismatch: %d", out.InputTokens)
	}
	if out.CacheReadTokens != 10 {
		t.Errorf("cache read tokens mismatch: %d", out.CacheReadTokens)
	}
	if out.Currency != money.USD {
		t.Errorf("currency mismatch: %q", out.Currency)
	}
	if out.CreatedAt != ts {
		t.Errorf("timestamp mismatch: %d vs %d", out.CreatedAt, ts)
	}
}

func TestBudgetEntryFromUsage_currencyDefaultsToUSD(t *testing.T) {
	out := BudgetEntryFromUsage(&BudgetEntryInput{
		AgentID: "agt_1", RunID: "run_1",
		// Currency intentionally blank, no PriceSnapshot
	})
	if out.Currency != money.USD {
		t.Errorf("expected USD default, got %q", out.Currency)
	}
}

func TestBudgetEntryFromUsage_currencyFromSnapshot(t *testing.T) {
	out := BudgetEntryFromUsage(&BudgetEntryInput{
		AgentID: "agt_1", RunID: "run_1",
		PriceSnapshot: &runtimemodel.PriceSnapshot{Currency: money.IRT},
	})
	if out.Currency != money.IRT {
		t.Errorf("expected IRT from snapshot, got %q", out.Currency)
	}
}

func TestBudgetEntryFromUsage_nilSafe(t *testing.T) {
	if BudgetEntryFromUsage(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestBudgetEntryToTokenUsage_nilSafe(t *testing.T) {
	if BudgetEntryToTokenUsage(nil) != nil {
		t.Fatal("expected nil for nil entry")
	}
}

func TestBudgetEntryToTokenUsage_roundTrip(t *testing.T) {
	entry := &runtimemodel.BudgetEntry{
		InputTokens:     200,
		OutputTokens:    100,
		CacheReadTokens: 5,
		Model:           "claude-3",
	}
	usage := BudgetEntryToTokenUsage(entry)
	if usage == nil {
		t.Fatal("expected non-nil")
	}
	if usage.Model != "claude-3" {
		t.Errorf("model mismatch: %q", usage.Model)
	}
	if usage.InputTokens != 200 {
		t.Errorf("input tokens mismatch: %d", usage.InputTokens)
	}
	if usage.CacheRead != 5 {
		t.Errorf("cache read mismatch: %d", usage.CacheRead)
	}
}

// ── ConnectorCredential ────────────────────────────────────────────────────

func TestConnectorCredentialUpsertToModel_fields(t *testing.T) {
	ts := int64(1_000_000)
	expires := int64(9_999_999)
	in := &ConnectorCredentialUpsert{
		ID:            "cred_1",
		ProjectID:     "prj_1",
		EnvID:         "env_1",
		ConnectorType: "openai",
		Label:         "primary",
		SourceType:    "encrypted",
		EncryptedBlob: []byte("secret"),
		KeyHash:       "abc123",
		CreatedBy:     "usr_1",
		CreatedAt:     &ts,
		ExpiresAt:     &expires,
	}
	out := ConnectorCredentialUpsertToModel(in)
	if out == nil {
		t.Fatal("expected non-nil")
	}
	if out.Status != controlmodel.CredStatusActive {
		t.Errorf("expected active status, got %q", out.Status)
	}
	if out.Version != 1 {
		t.Errorf("expected version 1, got %d", out.Version)
	}
	// EncryptedBlob must be present in the model (not stripped at this layer)
	if string(out.EncryptedBlob) != "secret" {
		t.Errorf("encrypted blob mismatch: %v", out.EncryptedBlob)
	}
}

func TestConnectorCredentialUpsertToModel_nilSafe(t *testing.T) {
	if ConnectorCredentialUpsertToModel(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestConnectorCredentialToView_stripsEncryptedBlob(t *testing.T) {
	cred := &controlmodel.ConnectorCredential{
		ID:            "cred_2",
		EncryptedBlob: []byte("top-secret"),
		Status:        controlmodel.CredStatusActive,
		Version:       2,
	}
	view := ConnectorCredentialToView(cred)
	if view == nil {
		t.Fatal("expected non-nil view")
	}
	// View must NOT expose EncryptedBlob
	// (ConnectorCredentialView has no EncryptedBlob field by design)
	// Verify other fields are mapped correctly
	if view.Status != controlmodel.CredStatusActive {
		t.Errorf("status mismatch: %q", view.Status)
	}
	if view.Version != 2 {
		t.Errorf("version mismatch: %d", view.Version)
	}
}

func TestConnectorCredentialToView_nilSafe(t *testing.T) {
	if ConnectorCredentialToView(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
}

// ── Org / User ─────────────────────────────────────────────────────────────

func TestOrgCreateToModel_fields(t *testing.T) {
	ts := int64(111)
	out := OrgCreateToModel(&OrgCreateInput{
		ID:        "org_1",
		Name:      "Acme",
		Slug:      "acme",
		Plan:      "pro",
		CreatedAt: &ts,
	})
	if out == nil {
		t.Fatal("expected non-nil")
	}
	if out.ID != "org_1" || out.Slug != "acme" || out.Plan != "pro" {
		t.Errorf("unexpected fields: %+v", out)
	}
	if out.CreatedAt != ts || out.UpdatedAt != ts {
		t.Errorf("timestamp mismatch: %d %d", out.CreatedAt, out.UpdatedAt)
	}
}

func TestOrgCreateToModel_nilSafe(t *testing.T) {
	if OrgCreateToModel(nil) != nil {
		t.Fatal("expected nil")
	}
}

func TestOrgToView_nilSafe(t *testing.T) {
	if OrgToView(nil) != nil {
		t.Fatal("expected nil")
	}
}

func TestOrgToView_fields(t *testing.T) {
	org := &controlmodel.Organization{ID: "org_2", Name: "Beta", Slug: "beta", Plan: "free", CreatedAt: 100, UpdatedAt: 200}
	v := OrgToView(org)
	if v.ID != "org_2" || v.Plan != "free" || v.CreatedAt != 100 {
		t.Errorf("unexpected view: %+v", v)
	}
}

func TestUserCreateToModel_defaultsStatus(t *testing.T) {
	out := UserCreateToModel(&UserCreateInput{ID: "usr_1", OrgID: "org_1", Email: "a@b.com"})
	if out == nil {
		t.Fatal("expected non-nil")
	}
	if out.Status != "active" {
		t.Errorf("expected active status, got %q", out.Status)
	}
}

func TestUserCreateToModel_nilSafe(t *testing.T) {
	if UserCreateToModel(nil) != nil {
		t.Fatal("expected nil")
	}
}

func TestUserToView_stripsPasswordHash(t *testing.T) {
	u := &controlmodel.User{
		ID:           "usr_2",
		PasswordHash: []byte("hashed-secret"),
		Status:       "active",
	}
	v := UserToView(u)
	// UserView has no PasswordHash field — verify other fields instead
	if v == nil {
		t.Fatal("expected non-nil view")
	}
	if v.ID != "usr_2" || v.Status != "active" {
		t.Errorf("unexpected view: %+v", v)
	}
}

func TestUserToView_nilSafe(t *testing.T) {
	if UserToView(nil) != nil {
		t.Fatal("expected nil")
	}
}

// ── Project ────────────────────────────────────────────────────────────────

func TestProjectCreateToModel_fields(t *testing.T) {
	ts := int64(222)
	out := ProjectCreateToModel(&ProjectCreateInput{
		ID:          "prj_1",
		OrgID:       "org_1",
		Name:        "Main",
		Slug:        "main",
		Description: "desc",
		CreatedAt:   &ts,
	})
	if out == nil || out.ID != "prj_1" || out.Slug != "main" {
		t.Errorf("unexpected: %+v", out)
	}
}

func TestProjectCreateToModel_nilSafe(t *testing.T) {
	if ProjectCreateToModel(nil) != nil {
		t.Fatal("expected nil")
	}
}

func TestProjectToView_nilSafe(t *testing.T) {
	if ProjectToView(nil) != nil {
		t.Fatal("expected nil")
	}
}

// ── Environment ────────────────────────────────────────────────────────────

func TestEnvironmentCreateToModel_fields(t *testing.T) {
	ts := int64(333)
	out := EnvironmentCreateToModel(&EnvironmentCreateInput{
		ID:        "env_1",
		ProjectID: "prj_1",
		Name:      "Production",
		Slug:      "prod",
		Type:      "production",
		CreatedAt: &ts,
	})
	if out == nil || out.ID != "env_1" || out.Type != "production" {
		t.Errorf("unexpected: %+v", out)
	}
	if out.CreatedAt != ts || out.UpdatedAt != ts {
		t.Errorf("timestamp mismatch: %d %d", out.CreatedAt, out.UpdatedAt)
	}
}

func TestEnvironmentCreateToModel_nilSafe(t *testing.T) {
	if EnvironmentCreateToModel(nil) != nil {
		t.Fatal("expected nil")
	}
}

func TestEnvironmentToView_nilSafe(t *testing.T) {
	if EnvironmentToView(nil) != nil {
		t.Fatal("expected nil")
	}
}

// ── Token ──────────────────────────────────────────────────────────────────

func TestTokenIssueToModel_fields(t *testing.T) {
	cap := money.MustParseAmount("1.00")
	ts := int64(444)
	out := TokenIssueToModel(&TokenIssue{
		ID:        "tok_1",
		AgentID:   "agt_1",
		Name:      "ci-token",
		BudgetCap: &cap,
		Scopes:    []string{"read"},
		CreatedAt: &ts,
	})
	if out == nil || out.ID != "tok_1" {
		t.Errorf("unexpected: %+v", out)
	}
	if out.BudgetCap == nil || *out.BudgetCap != cap {
		t.Errorf("budget cap mismatch: %v", out.BudgetCap)
	}
	if len(out.Scopes) != 1 || out.Scopes[0] != "read" {
		t.Errorf("scopes mismatch: %v", out.Scopes)
	}
}

func TestTokenIssueToModel_nilSafe(t *testing.T) {
	if TokenIssueToModel(nil) != nil {
		t.Fatal("expected nil")
	}
}

func TestAgentTokenToView_budgetFormatted(t *testing.T) {
	cap := money.MustParseAmount("2.50")
	token := &controlmodel.AgentToken{
		ID:        "tok_2",
		AgentID:   "agt_1",
		BudgetCap: &cap,
	}
	v := AgentTokenToView(token)
	if v == nil {
		t.Fatal("expected non-nil view")
	}
	if v.BudgetCap == nil || *v.BudgetCap == "" {
		t.Error("expected formatted budget cap string")
	}
}

func TestAgentTokenToView_nilBudget(t *testing.T) {
	token := &controlmodel.AgentToken{ID: "tok_3", AgentID: "agt_1"}
	v := AgentTokenToView(token)
	if v.BudgetCap != nil {
		t.Errorf("expected nil BudgetCap in view, got %v", v.BudgetCap)
	}
}

func TestAgentTokenToView_nilSafe(t *testing.T) {
	if AgentTokenToView(nil) != nil {
		t.Fatal("expected nil")
	}
}

// ── PriceBook ──────────────────────────────────────────────────────────────

func TestPriceBookRoundTrip_ModelToAppToModel(t *testing.T) {
	original := &runtimemodel.PriceBook{
		Version: "v2",
		Entries: []runtimemodel.PriceModel{
			{
				Provider:         "anthropic",
				Match:            "claude-3-5",
				Source:           "manual",
				Currency:         money.USD,
				InputPerMillion:  money.MustParseAmount("3.00"),
				OutputPerMillion: money.MustParseAmount("15.00"),
			},
		},
	}
	app := ModelPriceBookToApp(original)
	if app == nil {
		t.Fatal("expected non-nil app view")
	}
	if app.Version != "v2" {
		t.Errorf("version mismatch: %q", app.Version)
	}
	if len(app.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(app.Entries))
	}
	if app.Entries[0].Provider != "anthropic" {
		t.Errorf("provider mismatch: %q", app.Entries[0].Provider)
	}

	back := AppPriceBookToModel(app)
	if back == nil {
		t.Fatal("expected non-nil model")
	}
	if back.Entries[0].InputPerMillion != money.MustParseAmount("3.00") {
		t.Errorf("input price mismatch: %v", back.Entries[0].InputPerMillion)
	}
	if back.Entries[0].OutputPerMillion != money.MustParseAmount("15.00") {
		t.Errorf("output price mismatch: %v", back.Entries[0].OutputPerMillion)
	}
}

func TestPriceBookRoundTrip_nilSafe(t *testing.T) {
	if ModelPriceBookToApp(nil) != nil {
		t.Fatal("expected nil for nil model book")
	}
	if AppPriceBookToModel(nil) != nil {
		t.Fatal("expected nil for nil app book")
	}
}

func TestPriceBookRoundTrip_invalidAmountParsesToZero(t *testing.T) {
	app := &AppPriceBook{
		Version: "v1",
		Entries: []AppPriceModel{
			{InputPerMillion: "not-a-number", OutputPerMillion: "15.00"},
		},
	}
	out := AppPriceBookToModel(app)
	if out.Entries[0].InputPerMillion != 0 {
		t.Errorf("expected 0 for invalid amount, got %v", out.Entries[0].InputPerMillion)
	}
	if out.Entries[0].OutputPerMillion != money.MustParseAmount("15.00") {
		t.Errorf("output price should still parse: %v", out.Entries[0].OutputPerMillion)
	}
}
