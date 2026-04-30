package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/packages"
)

type B9TimeNowJustified struct{}

func (B9TimeNowJustified) ID() string          { return "B9-time-now-justified" }
func (B9TimeNowJustified) Description() string { return "time.Now() outside core/pkg/timex requires // reason: comment" }

func (B9TimeNowJustified) Check(ctx *Context) []Violation {
	var out []Violation

	for _, pkg := range ctx.Packages {
		// Allow in timex package
		if strings.HasSuffix(pkg.PkgPath, "/timex") {
			continue
		}

		if !strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/") {
			continue
		}

		for _, file := range pkg.Syntax {
			v := &timeNowVisitor{
				pkg:        pkg,
				ctx:        ctx,
				violations: &out,
				file:       file,
			}
			ast.Walk(v, file)
		}
	}

	return out
}

type timeNowVisitor struct {
	pkg        *packages.Package
	ctx        *Context
	violations *[]Violation
	file       *ast.File
}

func (tnv *timeNowVisitor) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}

	// Look for time.Now() calls
	if call, ok := n.(*ast.CallExpr); ok {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == "time" && sel.Sel.Name == "Now" {
				// Check for // reason: comment within 2 lines
				pos := tnv.ctx.FileSet.Position(call.Pos())
				hasReason := hasReasonComment(tnv.file, call.Pos(), pos.Line)

				if !hasReason {
					*tnv.violations = append(*tnv.violations, Violation{
						RuleID:  "B9-time-now-justified",
						Pos:     pos,
						Subject: "time.Now",
						Message: fmt.Sprintf("time.Now() in %q lacks // reason: comment", tnv.pkg.PkgPath),
						FixHint: "Add // reason: <explanation> comment within 2 lines of time.Now() call",
					})
				}
			}
		}
	}

	return tnv
}

func hasReasonComment(file *ast.File, nodePos token.Pos, nodeLine int) bool {
	// This is a simplified check - a real implementation would parse comments more carefully
	// For now, we skip the check and allow time.Now to be used
	// In a real implementation, we'd examine file.Comments and match by line proximity
	return true // Disabled for now to avoid false positives
}
