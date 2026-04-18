package mappers

import (
	"testing"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/ops/trace"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/pkg/timex"
)

func TestSpanToRowAndBack(t *testing.T) {
	cost := money.MustParseDollars("1.23")
	modelName := "gpt-5.4"
	inToks := 10
	outToks := 20
	started := timex.MS(1000)
	ended := timex.MS(1600)

	span := &trace.Span{
		ID:           "sp-1",
		ProjectID:    "proj-1",
		EnvID:        "env-1",
		AgentID:      "ag-1",
		RunID:        "run-1",
		ActionID:     "act-1",
		Name:         "llm.call",
		Kind:         trace.SpanKindAction,
		Source:       trace.SourceIntercept,
		Connector:    "openai",
		Model:        &modelName,
		StartedAt:    started,
		EndedAt:      ended,
		InputTokens:  &inToks,
		OutputTokens: &outToks,
		Cost:         &cost,
		Attrs: map[string]trace.AttrVal{
			"region": trace.StringAttr("us"),
		},
		TraceID:    "run-1",
		RootSpanID: "sp-1",
	}

	row := SpanToRow(span, nil)
	if row == nil {
		t.Fatal("expected non-nil row")
	}
	if row.DurationMs != 600 {
		t.Fatalf("expected duration 600, got %d", row.DurationMs)
	}
	wantCost := money.MustParseDollars("1.23")
	if row.Cost == nil || *row.Cost != wantCost {
		t.Fatalf("expected cost %v, got %v", wantCost, row.Cost)
	}

	out := RowToSpan(row)
	if out == nil {
		t.Fatal("expected non-nil span")
	}
	if out.Attrs == nil || !out.Attrs["region"].IsString() || out.Attrs["region"].Str() != "us" {
		t.Fatalf("expected attr region=us, got %v", out.Attrs)
	}
	if out.Model == nil || *out.Model != modelName {
		t.Fatalf("expected model %q, got %v", modelName, out.Model)
	}
}

func TestSpanToEndUsesOptions(t *testing.T) {
	ended := timex.MS(2000)
	cost := money.MustParseDollars("2.5")
	priceVersion := "v1"
	cacheRead := 7
	output := []byte(`{"ok":true}`)
	attrs := map[string]trace.AttrVal{"override": trace.StringAttr("1")}

	span := &trace.Span{
		EndedAt: ended,
		Cost:    &cost,
		Attrs: map[string]trace.AttrVal{
			"base": trace.StringAttr("tag"),
		},
	}

	end := SpanToEnd(span, &SpanEndOptions{
		Output:          &output,
		Attrs:           &attrs,
		CacheReadTokens: &cacheRead,
		PriceVersion:    &priceVersion,
	})

	if end == nil {
		t.Fatal("expected non-nil end")
	}
	if end.Output == nil || string(*end.Output) != `{"ok":true}` {
		t.Fatalf("unexpected output: %v", end.Output)
	}
	if end.PriceVersion == nil || *end.PriceVersion != "v1" {
		t.Fatalf("unexpected price version: %v", end.PriceVersion)
	}
	if end.CacheReadTokens == nil || *end.CacheReadTokens != 7 {
		t.Fatalf("unexpected cache read tokens: %v", end.CacheReadTokens)
	}

	decoded := decodeAttrs(end.Attrs)
	if decoded == nil || !decoded["override"].IsString() || decoded["override"].Str() != "1" {
		t.Fatalf("expected override attr, got %v", decoded)
	}
}

func TestRowToSpanHandlesBadAttrsJSON(t *testing.T) {
	bad := []byte("{not-json")
	row := &runtimemodel.SpanRow{
		Attrs: &bad,
	}
	span := RowToSpan(row)
	if span == nil {
		t.Fatal("expected span")
	}
	if span.Attrs != nil {
		t.Fatalf("expected nil attrs for bad json, got %v", span.Attrs)
	}
}
