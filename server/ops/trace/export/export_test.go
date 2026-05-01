package export

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/server/ops/trace"
)

// fixture builds a 5-span tree:
//
//	root
//	├── child-a
//	│   └── grandchild-a1
//	└── child-b (error)
//	    └── grandchild-b1
func fixture() (*trace.Node, []*runtimemodel.SpanRow) {
	ptr := func(s string) *string { return &s }
	errStr := "upstream timeout"
	spans := []*runtimemodel.SpanRow{
		{ID: "spn-root", Name: "root-action", Kind: "action", Connector: "openai", StartedAt: 1000, DurationMs: 200},
		{ID: "spn-a", ParentID: ptr("spn-root"), Name: "child-a", Kind: "llm", Connector: "openai", StartedAt: 1010, DurationMs: 80},
		{ID: "spn-a1", ParentID: ptr("spn-a"), Name: "grandchild-a1", Kind: "tool", Connector: "openai", StartedAt: 1020, DurationMs: 30},
		{ID: "spn-b", ParentID: ptr("spn-root"), Name: "child-b", Kind: "llm", Connector: "anthropic", StartedAt: 1090, DurationMs: 100, Error: &errStr},
		{ID: "spn-b1", ParentID: ptr("spn-b"), Name: "grandchild-b1", Kind: "tool", Connector: "anthropic", StartedAt: 1100, DurationMs: 20},
	}
	root, err := trace.BuildTree(spans)
	if err != nil {
		panic("fixture: " + err.Error())
	}
	return root, spans
}

// ---- DOT tests ----

func TestDOT_containsNodeIDs(t *testing.T) {
	root, _ := fixture()
	out, err := DOT(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"spn-root", "spn-a", "spn-a1", "spn-b", "spn-b1"} {
		if !bytes.Contains(out, []byte(id)) {
			t.Errorf("DOT output missing span ID %q", id)
		}
	}
}

func TestDOT_startsWithDigraph(t *testing.T) {
	root, _ := fixture()
	out, _ := DOT(root)
	if !bytes.HasPrefix(out, []byte("digraph trace {")) {
		t.Fatalf("DOT output should start with 'digraph trace {', got: %q", out[:min(len(out), 30)])
	}
}

func TestDOT_errorSpanHighlighted(t *testing.T) {
	root, _ := fixture()
	out, _ := DOT(root)
	// child-b has an error — should have color=red
	if !bytes.Contains(out, []byte("color=red")) {
		t.Error("error span must be colored red in DOT output")
	}
}

func TestDOT_nilRootSafe(t *testing.T) {
	out, err := DOT(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("digraph trace {")) {
		t.Error("DOT(nil) should return valid empty graph")
	}
}

func TestDOT_containsEdges(t *testing.T) {
	root, _ := fixture()
	out, _ := DOT(root)
	// Should have at least one -> edge
	if !bytes.Contains(out, []byte("->")) {
		t.Error("DOT output should contain directed edges")
	}
}

func TestDOT_hasCostLabel(t *testing.T) {
	cost := money.MustParseAmount("0.05")
	root := &trace.Node{
		Span: &runtimemodel.SpanRow{ID: "spn-root", Name: "root", Cost: &cost},
	}
	out, _ := DOT(root)
	if !bytes.Contains(out, []byte("$")) {
		t.Error("DOT label should include cost formatted with $")
	}
}

// ---- JSONL tests ----

func TestJSONL_eachSpanOneLine(t *testing.T) {
	_, spans := fixture()
	out, err := JSONL(spans)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimRight(out, "\n"), []byte("\n"))
	if len(lines) != len(spans) {
		t.Fatalf("expected %d lines, got %d", len(spans), len(lines))
	}
}

func TestJSONL_validJSON(t *testing.T) {
	_, spans := fixture()
	out, _ := JSONL(spans)
	for i, line := range bytes.Split(bytes.TrimRight(out, "\n"), []byte("\n")) {
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			t.Fatalf("line %d is not valid JSON: %v\n%s", i, err, line)
		}
	}
}

func TestJSONL_spanIDsPresent(t *testing.T) {
	_, spans := fixture()
	out, _ := JSONL(spans)
	for _, span := range spans {
		if !bytes.Contains(out, []byte(span.ID)) {
			t.Errorf("JSONL missing span ID %q", span.ID)
		}
	}
}

func TestJSONL_nilSpansSkipped(t *testing.T) {
	spans := []*runtimemodel.SpanRow{
		nil,
		{ID: "spn-1", Name: "real"},
		nil,
	}
	out, err := JSONL(spans)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimRight(out, "\n"), []byte("\n"))
	// only one real span
	nonEmpty := 0
	for _, l := range lines {
		if len(bytes.TrimSpace(l)) > 0 {
			nonEmpty++
		}
	}
	if nonEmpty != 1 {
		t.Fatalf("expected 1 non-empty line, got %d", nonEmpty)
	}
}

func TestJSONL_empty(t *testing.T) {
	out, err := JSONL(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("JSONL(nil) should return empty output, got %q", out)
	}
}

// ---- Mermaid tests ----

func TestMermaid_containsSequenceDiagram(t *testing.T) {
	root, _ := fixture()
	out, err := Mermaid(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("sequenceDiagram")) {
		t.Error("Mermaid output should contain 'sequenceDiagram'")
	}
}

func TestMermaid_errorArrow(t *testing.T) {
	root, _ := fixture()
	out, _ := Mermaid(root)
	// child-b has an error — should use -x arrow
	if !bytes.Contains(out, []byte("-x")) {
		t.Error("Mermaid output should use -x arrow for error spans")
	}
}

func TestMermaid_happyArrow(t *testing.T) {
	root, _ := fixture()
	out, _ := Mermaid(root)
	// should also contain ->> arrows for success spans
	if !bytes.Contains(out, []byte("->>")) {
		t.Error("Mermaid output should use ->> arrow for success spans")
	}
}

func TestMermaid_participantsListed(t *testing.T) {
	root, _ := fixture()
	out, _ := Mermaid(root)
	s := string(out)
	if !strings.Contains(s, "participant") {
		t.Error("Mermaid output should declare participants")
	}
}

func TestMermaid_spanNamesPresent(t *testing.T) {
	root, _ := fixture()
	out, _ := Mermaid(root)
	for _, name := range []string{"child-a", "grandchild-a1", "child-b", "grandchild-b1"} {
		if !bytes.Contains(out, []byte(name)) {
			t.Errorf("Mermaid output missing span name %q", name)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
