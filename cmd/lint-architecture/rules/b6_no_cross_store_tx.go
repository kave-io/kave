package rules

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/packages"
)

type B6NoCrossStoreTx struct{}

func (B6NoCrossStoreTx) ID() string { return "B6-no-cross-store-tx" }
func (B6NoCrossStoreTx) Description() string {
	return "WithTx closures must not reference SpanStore or AuditStore"
}

func (B6NoCrossStoreTx) Check(ctx *Context) []Violation {
	var out []Violation

	for _, pkg := range ctx.Packages {
		if !strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/") {
			continue
		}

		for _, file := range pkg.Syntax {
			v := &txVisitor{
				pkg:        pkg,
				ctx:        ctx,
				violations: &out,
			}
			ast.Walk(v, file)
		}
	}

	return out
}

type txVisitor struct {
	pkg        *packages.Package
	ctx        *Context
	violations *[]Violation
}

func (tv *txVisitor) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}
	if call, ok := n.(*ast.CallExpr); ok {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "WithTx" {
			if len(call.Args) > 0 {
				if fn, ok := call.Args[0].(*ast.FuncLit); ok {
					if referencesForbiddenStore(fn.Body) {
						pos := tv.ctx.FileSet.Position(call.Pos())
						*tv.violations = append(*tv.violations, Violation{
							RuleID:  "B6-no-cross-store-tx",
							Pos:     pos,
							Subject: "WithTx",
							Message: fmt.Sprintf("WithTx closure in %q references SpanStore or AuditStore", tv.pkg.PkgPath),
							FixHint: "Span and audit writes must happen after the transaction commits",
						})
					}
				}
			}
		}
	}

	return tv
}

func referencesForbiddenStore(block *ast.BlockStmt) bool {
	for _, stmt := range block.List {
		if containsForbiddenReference(stmt) {
			return true
		}
	}
	return false
}

func containsForbiddenReference(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok {
			if strings.Contains(ident.Name, "SpanStore") || strings.Contains(ident.Name, "AuditStore") {
				found = true
				return false
			}
		}
		return true
	})
	return found
}
