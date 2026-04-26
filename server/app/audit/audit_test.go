package audit

import (
	"context"
	"os"
	"testing"
	"time"

	auditmodel "github.com/kave-io/kave/core/model/audit"
	corestore "github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/store/sqlite"
)

func TestAppendAudit(t *testing.T) {
	ctx := context.Background()
	tempFile, err := os.CreateTemp(t.TempDir(), "audit_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tempFile.Close()

	auditStore, err := sqlite.NewAuditStoreFromPath(tempFile.Name())
	if err != nil {
		t.Fatalf("failed to create audit store: %v", err)
	}
	defer auditStore.Close()

	entry := &auditmodel.AuditLog{
		ID:           "aud_test1",
		OrgID:        "org_123",
		ActorID:      "user_456",
		ActorType:    "user",
		Event:        "policy.created",
		ResourceType: "policy",
		ResourceID:   "pol_789",
		CreatedAt:    time.Now().UnixMilli(),
	}

	err = auditStore.AppendAudit(ctx, entry)
	if err != nil {
		t.Fatalf("AppendAudit failed: %v", err)
	}
}

func TestQueryAudits(t *testing.T) {
	ctx := context.Background()
	tempFile, err := os.CreateTemp(t.TempDir(), "audit_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tempFile.Close()

	auditStore, err := sqlite.NewAuditStoreFromPath(tempFile.Name())
	if err != nil {
		t.Fatalf("failed to create audit store: %v", err)
	}
	defer auditStore.Close()

	now := time.Now().UnixMilli()

	// Append multiple entries
	entries := []*auditmodel.AuditLog{
		{
			ID:           "aud_1",
			OrgID:        "org_123",
			ActorID:      "user_456",
			ActorType:    "user",
			Event:        "policy.created",
			ResourceType: "policy",
			ResourceID:   "pol_1",
			CreatedAt:    now,
		},
		{
			ID:           "aud_2",
			OrgID:        "org_123",
			ActorID:      "user_456",
			ActorType:    "user",
			Event:        "token.revoked",
			ResourceType: "token",
			ResourceID:   "tok_2",
			CreatedAt:    now + 1000,
		},
		{
			ID:           "aud_3",
			OrgID:        "org_456",
			ActorID:      "user_999",
			ActorType:    "user",
			Event:        "policy.created",
			ResourceType: "policy",
			ResourceID:   "pol_3",
			CreatedAt:    now + 2000,
		},
	}

	for _, entry := range entries {
		if err := auditStore.AppendAudit(ctx, entry); err != nil {
			t.Fatalf("AppendAudit failed: %v", err)
		}
	}

	// Test: query all for org_123
	result, err := auditStore.QueryAudits(ctx, &auditmodel.AuditFilter{OrgID: "org_123"}, corestore.Page{Limit: 100})
	if err != nil {
		t.Fatalf("QueryAudits failed: %v", err)
	}
	if len(result.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(result.Items))
	}

	// Test: query by actor
	result, err = auditStore.QueryAudits(ctx, &auditmodel.AuditFilter{OrgID: "org_123", ActorID: "user_456"}, corestore.Page{Limit: 100})
	if err != nil {
		t.Fatalf("QueryAudits by actor failed: %v", err)
	}
	if len(result.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(result.Items))
	}

	// Test: query by resource type
	result, err = auditStore.QueryAudits(ctx, &auditmodel.AuditFilter{OrgID: "org_123", ResourceType: "policy"}, corestore.Page{Limit: 100})
	if err != nil {
		t.Fatalf("QueryAudits by resource type failed: %v", err)
	}
	if len(result.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(result.Items))
	}

	// Test: query by time range
	result, err = auditStore.QueryAudits(ctx, &auditmodel.AuditFilter{
		OrgID:  "org_123",
		FromMs: &now,
		ToMs:   ptrInt64(now + 500),
	}, corestore.Page{Limit: 100})
	if err != nil {
		t.Fatalf("QueryAudits by time range failed: %v", err)
	}
	if len(result.Items) != 1 {
		t.Errorf("expected 1 item in time range, got %d", len(result.Items))
	}

	// Test: query empty result
	result, err = auditStore.QueryAudits(ctx, &auditmodel.AuditFilter{OrgID: "nonexistent"}, corestore.Page{Limit: 100})
	if err != nil {
		t.Fatalf("QueryAudits empty result failed: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(result.Items))
	}
}

func ptrInt64(v int64) *int64 {
	return &v
}
