package export

import (
	"bytes"
	"fmt"
	"html"
	"strings"

	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/server/ops/trace"
)

// DOT encodes a trace tree as Graphviz DOT.
func DOT(root *trace.Node) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("digraph trace {\n")
	buf.WriteString("  rankdir=LR;\n")
	buf.WriteString("  node [shape=box, style=rounded];\n")

	var walk func(*trace.Node)
	walk = func(n *trace.Node) {
		if n == nil || n.Span == nil {
			return
		}
		label := fmt.Sprintf("%s\\n%dms\\n%s", n.Span.Name, n.Span.DurationMs, formatCost(n.Span.Cost))
		buf.WriteString("  ")
		buf.WriteString(dotID(n.Span.ID))
		buf.WriteString(" [label=\"")
		buf.WriteString(strings.ReplaceAll(html.EscapeString(label), "\"", "\\\""))
		buf.WriteString("\"")
		if n.Span.Error != nil {
			buf.WriteString(", color=red, fontcolor=red")
		}
		buf.WriteString("];\n")
		for _, child := range n.Children {
			if child == nil || child.Span == nil {
				continue
			}
			buf.WriteString("  ")
			buf.WriteString(dotID(n.Span.ID))
			buf.WriteString(" -> ")
			buf.WriteString(dotID(child.Span.ID))
			buf.WriteString(";\n")
			walk(child)
		}
	}
	walk(root)
	buf.WriteString("}\n")
	return buf.Bytes(), nil
}

func dotID(v string) string {
	if v == "" {
		return "\"trace\""
	}
	return fmt.Sprintf("%q", v)
}

func formatCost(v *money.Amount) string {
	if v == nil {
		return "$0"
	}
	return "$" + v.String()
}
