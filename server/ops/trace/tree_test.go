package trace

import (
	"testing"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
)

func TestBuildTree(t *testing.T) {
	rootParent := ""
	childParent := "root"
	spans := []*runtimemodel.SpanRow{
		{ID: "child-b", ParentID: &childParent, StartedAt: 3},
		{ID: "root", StartedAt: 1},
		{ID: "child-a", ParentID: &childParent, StartedAt: 2},
	}

	tree, err := BuildTree(spans)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	if tree.Span.ID != "root" {
		t.Fatalf("root = %q", tree.Span.ID)
	}
	if len(tree.Children) != 2 {
		t.Fatalf("children = %d", len(tree.Children))
	}
	if tree.Children[0].Span.ID != "child-a" || tree.Children[1].Span.ID != "child-b" {
		t.Fatalf("children order = %q, %q", tree.Children[0].Span.ID, tree.Children[1].Span.ID)
	}
	_ = rootParent
}

func TestBuildTreeOrphan(t *testing.T) {
	parent := "missing"
	_, err := BuildTree([]*runtimemodel.SpanRow{
		{ID: "root", StartedAt: 1},
		{ID: "child", ParentID: &parent, StartedAt: 2},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
