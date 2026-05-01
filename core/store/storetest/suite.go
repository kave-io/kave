// Package storetest provides a shared test suite for store.SpanStore implementations.
// Integration tests in server/internal/store/duckdb/ and server/internal/store/sqlite/
// call RunSpanSuite(t, store) to exercise all SpanStore contract requirements.
package storetest

import (
	"context"
	"testing"
	"time"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
)

// RunSpanSuite runs the full SpanStore contract suite against the provided implementation.
// The store must already be migrated before calling this.
func RunSpanSuite(t *testing.T, s store.SpanStore) {
	t.Helper()
	t.Run("OpenAndGetSpan", func(t *testing.T) { testOpenAndGetSpan(t, s) })
	t.Run("CloseSpan", func(t *testing.T) { testCloseSpan(t, s) })
	t.Run("QuerySpans_ByRunID", func(t *testing.T) { testQuerySpansByRunID(t, s) })
	t.Run("QuerySpans_ByAgentID", func(t *testing.T) { testQuerySpansByAgentID(t, s) })
	t.Run("QuerySpans_HasError", func(t *testing.T) { testQuerySpansHasError(t, s) })
	t.Run("QuerySpans_DateRange", func(t *testing.T) { testQuerySpansDateRange(t, s) })
	t.Run("QuerySpans_Pagination", func(t *testing.T) { testQuerySpansPagination(t, s) })
	t.Run("SpendByDimension_Connector", func(t *testing.T) { testSpendByDimensionConnector(t, s) })
	t.Run("SpendByDimension_Agent", func(t *testing.T) { testSpendByDimensionAgent(t, s) })
	t.Run("OpenSpan_DuplicateID", func(t *testing.T) { testOpenSpanDuplicateID(t, s) })
	t.Run("GetSpan_NotFound", func(t *testing.T) { testGetSpanNotFound(t, s) })
	t.Run("CloseSpan_NotFound_NoError", func(t *testing.T) { testCloseSpanNotFound(t, s) })
}

// ── helpers ─────────────────────────────────────────────────────────────────

func ptr[T any](v T) *T { return &v }

func nowMs() int64 { return time.Now().UnixMilli() }

// seed inserts a basic open span and returns it.
func seed(t *testing.T, s store.SpanStore, id, runID, agentID, connector string, cost money.Amount) *runtimemodel.SpanRow {
	t.Helper()
	row := &runtimemodel.SpanRow{
		ID:        id,
		RunID:     runID,
		AgentID:   agentID,
		ProjectID: "prj_test",
		EnvID:     "env_test",
		Name:      "test-span-" + id,
		Kind:      "llm",
		Connector: connector,
		StartedAt: nowMs(),
		CreatedAt: nowMs(),
		Cost:      &cost,
	}
	if err := s.OpenSpan(context.Background(), row); err != nil {
		t.Fatalf("OpenSpan(%s): %v", id, err)
	}
	return row
}

// ── tests ────────────────────────────────────────────────────────────────────

func testOpenAndGetSpan(t *testing.T, s store.SpanStore) {
	cost := money.MustParseAmount("0.05")
	seed(t, s, "spn-og-1", "run-og-1", "agt-og-1", "openai", cost)

	got, err := s.GetSpan(context.Background(), "spn-og-1")
	if err != nil {
		t.Fatalf("GetSpan: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil span")
	}
	if got.ID != "spn-og-1" {
		t.Errorf("ID mismatch: %q", got.ID)
	}
	if got.RunID != "run-og-1" {
		t.Errorf("RunID mismatch: %q", got.RunID)
	}
	if got.Cost == nil || *got.Cost != cost {
		t.Errorf("Cost mismatch: %v", got.Cost)
	}
}

func testCloseSpan(t *testing.T, s store.SpanStore) {
	cost := money.MustParseAmount("0.10")
	seed(t, s, "spn-cl-1", "run-cl-1", "agt-cl-1", "anthropic", cost)

	end := &runtimemodel.SpanEnd{
		EndedAt:      ptr(nowMs()),
		DurationMs:   150,
		InputTokens:  ptr(200),
		OutputTokens: ptr(100),
		Cost:         ptr(money.MustParseAmount("0.15")),
	}
	if err := s.CloseSpan(context.Background(), "spn-cl-1", end); err != nil {
		t.Fatalf("CloseSpan: %v", err)
	}

	got, err := s.GetSpan(context.Background(), "spn-cl-1")
	if err != nil {
		t.Fatalf("GetSpan after close: %v", err)
	}
	if got.EndedAt == nil {
		t.Error("EndedAt should be set after close")
	}
	if got.DurationMs != 150 {
		t.Errorf("DurationMs mismatch: %d", got.DurationMs)
	}
	// Cost from SpanEnd should override the open cost
	if got.Cost == nil || *got.Cost != money.MustParseAmount("0.15") {
		t.Errorf("Cost should be updated after close: %v", got.Cost)
	}
}

func testQuerySpansByRunID(t *testing.T, s store.SpanStore) {
	cost := money.Amount(0)
	seed(t, s, "spn-qr-1", "run-qr-unique", "agt-qr-1", "openai", cost)
	seed(t, s, "spn-qr-2", "run-qr-unique", "agt-qr-1", "openai", cost)
	seed(t, s, "spn-qr-3", "run-other", "agt-qr-1", "openai", cost)

	res, err := s.QuerySpans(context.Background(), &runtimemodel.SpanFilter{RunID: "run-qr-unique"}, store.Page{Limit: 50})
	if err != nil {
		t.Fatalf("QuerySpans: %v", err)
	}
	if len(res.Items) < 2 {
		t.Errorf("expected at least 2 spans for run-qr-unique, got %d", len(res.Items))
	}
	for _, sp := range res.Items {
		if sp.RunID != "run-qr-unique" {
			t.Errorf("unexpected run ID %q in results", sp.RunID)
		}
	}
}

func testQuerySpansByAgentID(t *testing.T, s store.SpanStore) {
	cost := money.Amount(0)
	seed(t, s, "spn-qa-1", "run-qa-1", "agt-qa-unique", "openai", cost)
	seed(t, s, "spn-qa-2", "run-qa-2", "agt-qa-unique", "openai", cost)
	seed(t, s, "spn-qa-3", "run-qa-3", "agt-qa-other", "openai", cost)

	res, err := s.QuerySpans(context.Background(), &runtimemodel.SpanFilter{AgentID: "agt-qa-unique"}, store.Page{Limit: 50})
	if err != nil {
		t.Fatalf("QuerySpans by agent: %v", err)
	}
	if len(res.Items) < 2 {
		t.Errorf("expected at least 2 spans for agt-qa-unique, got %d", len(res.Items))
	}
}

func testQuerySpansHasError(t *testing.T, s store.SpanStore) {
	cost := money.Amount(0)
	seed(t, s, "spn-he-1", "run-he-1", "agt-he-1", "openai", cost)
	// Close with an error
	errStr := "upstream timeout"
	if err := s.CloseSpan(context.Background(), "spn-he-1", &runtimemodel.SpanEnd{
		Error: &errStr, DurationMs: 1,
	}); err != nil {
		t.Fatalf("CloseSpan with error: %v", err)
	}

	// Normal span
	seed(t, s, "spn-he-2", "run-he-2", "agt-he-1", "openai", cost)
	if err := s.CloseSpan(context.Background(), "spn-he-2", &runtimemodel.SpanEnd{DurationMs: 1}); err != nil {
		t.Fatalf("CloseSpan: %v", err)
	}

	hasErr := true
	res, err := s.QuerySpans(context.Background(), &runtimemodel.SpanFilter{AgentID: "agt-he-1", HasError: &hasErr}, store.Page{Limit: 50})
	if err != nil {
		t.Fatalf("QuerySpans HasError: %v", err)
	}
	for _, sp := range res.Items {
		if sp.Error == nil {
			t.Errorf("HasError filter returned span %q without an error", sp.ID)
		}
	}
}

func testQuerySpansDateRange(t *testing.T, s store.SpanStore) {
	now := nowMs()
	past := now - 10_000 // 10 seconds ago
	future := now + 10_000

	cost := money.Amount(0)
	seed(t, s, "spn-dr-1", "run-dr-1", "agt-dr-1", "openai", cost)

	// Filter to include the span
	res, err := s.QuerySpans(context.Background(), &runtimemodel.SpanFilter{
		AgentID: "agt-dr-1",
		FromMs:  &past,
		ToMs:    &future,
	}, store.Page{Limit: 50})
	if err != nil {
		t.Fatalf("QuerySpans date range: %v", err)
	}
	found := false
	for _, sp := range res.Items {
		if sp.ID == "spn-dr-1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("span should be within date range but was not returned")
	}
}

func testQuerySpansPagination(t *testing.T, s store.SpanStore) {
	cost := money.Amount(0)
	const total = 5
	for i := 0; i < total; i++ {
		id := "spn-pg-" + string(rune('a'+i))
		seed(t, s, id, "run-pg-paginate", "agt-pg-1", "openai", cost)
	}

	// First page: limit 2
	res1, err := s.QuerySpans(context.Background(), &runtimemodel.SpanFilter{RunID: "run-pg-paginate"}, store.Page{Limit: 2})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(res1.Items) != 2 {
		t.Fatalf("expected 2 items on first page, got %d", len(res1.Items))
	}
	if res1.NextCursor == "" {
		t.Error("expected a next cursor after first page")
	}

	// Second page
	res2, err := s.QuerySpans(context.Background(), &runtimemodel.SpanFilter{RunID: "run-pg-paginate"}, store.Page{Limit: 2, Cursor: res1.NextCursor})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(res2.Items) == 0 {
		t.Error("expected items on second page")
	}
	// No ID overlap between pages
	seen := map[string]struct{}{}
	for _, sp := range res1.Items {
		seen[sp.ID] = struct{}{}
	}
	for _, sp := range res2.Items {
		if _, dup := seen[sp.ID]; dup {
			t.Errorf("duplicate span %q across pages", sp.ID)
		}
	}
}

func testSpendByDimensionConnector(t *testing.T, s store.SpanStore) {
	oaiCost := money.MustParseAmount("0.30")
	antCost := money.MustParseAmount("0.20")
	seed(t, s, "spn-sd-1", "run-sd-1", "agt-sd-1", "openai", oaiCost)
	seed(t, s, "spn-sd-2", "run-sd-2", "agt-sd-1", "anthropic", antCost)

	if err := s.CloseSpan(context.Background(), "spn-sd-1", &runtimemodel.SpanEnd{Cost: &oaiCost, DurationMs: 1}); err != nil {
		t.Fatalf("CloseSpan sd-1: %v", err)
	}
	if err := s.CloseSpan(context.Background(), "spn-sd-2", &runtimemodel.SpanEnd{Cost: &antCost, DurationMs: 1}); err != nil {
		t.Fatalf("CloseSpan sd-2: %v", err)
	}

	spend, err := s.SpendByDimension(context.Background(), "connector", &runtimemodel.SpanFilter{AgentID: "agt-sd-1"})
	if err != nil {
		t.Fatalf("SpendByDimension: %v", err)
	}
	if _, ok := spend["openai"]; !ok {
		t.Error("expected spend entry for openai")
	}
	if _, ok := spend["anthropic"]; !ok {
		t.Error("expected spend entry for anthropic")
	}
	if spend["openai"] != oaiCost {
		t.Errorf("openai spend mismatch: %v vs %v", spend["openai"], oaiCost)
	}
}

func testSpendByDimensionAgent(t *testing.T, s store.SpanStore) {
	c1 := money.MustParseAmount("0.10")
	c2 := money.MustParseAmount("0.05")
	seed(t, s, "spn-sda-1", "run-sda-1", "agt-sda-alpha", "openai", c1)
	seed(t, s, "spn-sda-2", "run-sda-2", "agt-sda-beta", "openai", c2)

	if err := s.CloseSpan(context.Background(), "spn-sda-1", &runtimemodel.SpanEnd{Cost: &c1, DurationMs: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CloseSpan(context.Background(), "spn-sda-2", &runtimemodel.SpanEnd{Cost: &c2, DurationMs: 1}); err != nil {
		t.Fatal(err)
	}

	spend, err := s.SpendByDimension(context.Background(), "agent_id", &runtimemodel.SpanFilter{ProjectID: "prj_test"})
	if err != nil {
		t.Fatalf("SpendByDimension agent: %v", err)
	}
	if spend["agt-sda-alpha"] == 0 {
		t.Error("expected non-zero spend for agt-sda-alpha")
	}
}

func testOpenSpanDuplicateID(t *testing.T, s store.SpanStore) {
	cost := money.Amount(0)
	seed(t, s, "spn-dup-1", "run-dup-1", "agt-dup-1", "openai", cost)

	row := &runtimemodel.SpanRow{
		ID:        "spn-dup-1", // same ID
		RunID:     "run-dup-1",
		AgentID:   "agt-dup-1",
		ProjectID: "prj_test",
		EnvID:     "env_test",
		Name:      "duplicate",
		StartedAt: nowMs(),
		CreatedAt: nowMs(),
	}
	err := s.OpenSpan(context.Background(), row)
	// A duplicate ID must either error or be idempotent — it must never silently
	// insert a second row that corrupts query results.
	if err == nil {
		// If no error, verify only one span exists with that ID
		res, qErr := s.QuerySpans(context.Background(), &runtimemodel.SpanFilter{ID: "spn-dup-1"}, store.Page{Limit: 10})
		if qErr != nil {
			t.Fatalf("QuerySpans after dup insert: %v", qErr)
		}
		if len(res.Items) > 1 {
			t.Errorf("duplicate insert created %d rows; must be exactly 1", len(res.Items))
		}
	}
}

func testGetSpanNotFound(t *testing.T, s store.SpanStore) {
	got, err := s.GetSpan(context.Background(), "spn-does-not-exist-xyz")
	// Either nil+nil (not found) or nil+err — the span must be nil.
	if got != nil {
		t.Errorf("expected nil span for missing ID, got %+v", got)
	}
	_ = err // some stores return an error, others return nil; both acceptable
}

func testCloseSpanNotFound(t *testing.T, s store.SpanStore) {
	// Closing a non-existent span should not panic.
	// Error is acceptable; a panic or data corruption is not.
	_ = s.CloseSpan(context.Background(), "spn-never-opened-xyz", &runtimemodel.SpanEnd{DurationMs: 1})
}
