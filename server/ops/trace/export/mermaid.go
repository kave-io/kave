package export

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/kave-io/kave/server/ops/trace"
)

// Mermaid encodes a trace tree as a sequenceDiagram.
func Mermaid(root *trace.Node) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("%%{init: {'themeCSS': '.error-line{stroke:#b00020;color:#b00020;}'}}%%\n")
	buf.WriteString("sequenceDiagram\n")

	participants := map[string]bool{}
	var declare func(*trace.Node)
	declare = func(n *trace.Node) {
		name := participantName(n)
		if !participants[name] {
			participants[name] = true
			buf.WriteString("participant ")
			buf.WriteString(name)
			if label := participantLabel(n); label != name {
				buf.WriteString(" as ")
				buf.WriteString(label)
			}
			buf.WriteByte('\n')
		}
		for _, child := range n.Children {
			declare(child)
		}
	}
	declare(root)

	var walk func(parent *trace.Node)
	walk = func(parent *trace.Node) {
		for _, child := range parent.Children {
			arrow := "->>"
			if child.Span != nil && child.Span.Error != nil {
				arrow = "-x"
			}
			buf.WriteString(participantName(parent))
			buf.WriteString(arrow)
			buf.WriteString(participantName(child))
			buf.WriteString(": ")
			buf.WriteString(strings.ReplaceAll(child.Span.Name, "\n", " "))
			buf.WriteByte('\n')
			walk(child)
		}
	}
	walk(root)
	return buf.Bytes(), nil
}

func participantName(n *trace.Node) string {
	if n == nil || n.Span == nil {
		return "trace"
	}
	if n.Span.Connector != "" {
		return sanitizeIdent(n.Span.Connector)
	}
	return sanitizeIdent(n.Span.ID)
}

func participantLabel(n *trace.Node) string {
	if n == nil || n.Span == nil {
		return "trace"
	}
	if n.Span.Connector != "" {
		return quoteLabel(n.Span.Connector)
	}
	return quoteLabel(n.Span.ID)
}

func sanitizeIdent(v string) string {
	v = strings.ToLower(v)
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "trace"
	}
	return b.String()
}

func quoteLabel(v string) string {
	return fmt.Sprintf("%q", v)
}
