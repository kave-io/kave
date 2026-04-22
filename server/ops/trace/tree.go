package trace

import (
	"errors"
	"fmt"
	"sort"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
)

// Node is a trace tree node rooted at a single span.
type Node struct {
	Span     *runtimemodel.SpanRow `json:"span"`
	Children []*Node               `json:"children,omitempty"`
}

// BuildTree builds a parent-child tree from a flat span list.
func BuildTree(spans []*runtimemodel.SpanRow) (*Node, error) {
	if len(spans) == 0 {
		return nil, errors.New("trace: no spans")
	}

	nodes := make(map[string]*Node, len(spans))
	parents := make(map[string]string, len(spans))
	var root *Node

	for _, span := range spans {
		if span == nil {
			return nil, errors.New("trace: nil span")
		}
		if span.ID == "" {
			return nil, fmt.Errorf("trace: span missing id")
		}
		if _, exists := nodes[span.ID]; exists {
			return nil, fmt.Errorf("trace: duplicate span %q", span.ID)
		}
		nodes[span.ID] = &Node{Span: span}
		if span.ParentID != nil && *span.ParentID != "" {
			parents[span.ID] = *span.ParentID
		}
	}

	for _, node := range nodes {
		parentID, hasParent := parents[node.Span.ID]
		if !hasParent {
			if root != nil {
				return nil, fmt.Errorf("trace: multiple roots %q and %q", root.Span.ID, node.Span.ID)
			}
			root = node
			continue
		}
		parent, ok := nodes[parentID]
		if !ok {
			return nil, fmt.Errorf("trace: orphan span %q parent %q", node.Span.ID, parentID)
		}
		parent.Children = append(parent.Children, node)
	}

	if root == nil {
		return nil, errors.New("trace: missing root span")
	}

	sortChildren(root)

	seen := make(map[string]bool, len(nodes))
	stack := make(map[string]bool, len(nodes))
	var visit func(n *Node) error
	visit = func(n *Node) error {
		if stack[n.Span.ID] {
			return fmt.Errorf("trace: cycle at %q", n.Span.ID)
		}
		if seen[n.Span.ID] {
			return nil
		}
		stack[n.Span.ID] = true
		seen[n.Span.ID] = true
		for _, child := range n.Children {
			if err := visit(child); err != nil {
				return err
			}
		}
		stack[n.Span.ID] = false
		return nil
	}
	if err := visit(root); err != nil {
		return nil, err
	}
	if len(seen) != len(nodes) {
		return nil, errors.New("trace: disconnected spans")
	}
	return root, nil
}

func sortChildren(node *Node) {
	sort.SliceStable(node.Children, func(i, j int) bool {
		li := node.Children[i].Span.StartedAt
		lj := node.Children[j].Span.StartedAt
		if li != lj {
			return li < lj
		}
		return node.Children[i].Span.ID < node.Children[j].Span.ID
	})
	for _, child := range node.Children {
		sortChildren(child)
	}
}
