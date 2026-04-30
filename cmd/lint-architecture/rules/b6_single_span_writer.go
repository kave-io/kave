package rules

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/packages"
)

type B6SingleSpanWriter struct{}

func (B6SingleSpanWriter) ID() string          { return "B6-single-span-writer" }
func (B6SingleSpanWriter) Description() string { return "OpenSpan/CloseSpan only in core/runtime/trace/* and *_test.go" }

func (B6SingleSpanWriter) Check(ctx *Context) []Violation {
	var out []Violation

	for _, pkg := range ctx.Packages {
		if !strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/") {
			continue
		}

		for _, file := range pkg.Syntax {
			v := &spanWriterVisitor{
				pkg:        pkg,
				ctx:        ctx,
				violations: &out,
				filename:   ctx.FileSet.Position(file.Pos()).Filename,
			}
			ast.Walk(v, file)
		}
	}

	return out
}

type spanWriterVisitor struct {
	pkg        *packages.Package
	ctx        *Context
	violations *[]Violation
	filename   string
}

func (sw *spanWriterVisitor) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}

	// Look for OpenSpan / CloseSpan calls
	if call, ok := n.(*ast.CallExpr); ok {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if sel.Sel.Name == "OpenSpan" || sel.Sel.Name == "CloseSpan" {
				// Allowed in trace/ or test files
				if strings.Contains(sw.filename, "/core/runtime/trace/") ||
					strings.HasSuffix(sw.filename, "_test.go") {
					return sw
				}

				pos := sw.ctx.FileSet.Position(call.Pos())
				*sw.violations = append(*sw.violations, Violation{
					RuleID:  "B6-single-span-writer",
					Pos:     pos,
					Subject: sel.Sel.Name,
					Message: fmt.Sprintf("%s called in %q outside trace or test", sel.Sel.Name, sw.pkg.PkgPath),
					FixHint: "Only core/runtime/trace/* may call OpenSpan/CloseSpan; D9 enforces single span writer",
				})
			}
		}
	}

	return sw
}
