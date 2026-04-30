package rules

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/packages"
)

type B8NoManualPrefix struct{}

func (B8NoManualPrefix) ID() string { return "B8-no-manual-prefix" }
func (B8NoManualPrefix) Description() string {
	return "All IDs from core/pkg/ids.New(); no manual prefix concat"
}

func (B8NoManualPrefix) Check(ctx *Context) []Violation {
	var out []Violation

	for _, pkg := range ctx.Packages {
		if strings.HasSuffix(pkg.PkgPath, "/ids") {
			continue
		}

		if !strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/") {
			continue
		}

		for _, file := range pkg.Syntax {
			v := &prefixVisitor{
				pkg:        pkg,
				ctx:        ctx,
				violations: &out,
			}
			ast.Walk(v, file)
		}
	}

	return out
}

type prefixVisitor struct {
	pkg        *packages.Package
	ctx        *Context
	violations *[]Violation
}

func (pv *prefixVisitor) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}
	if binOp, ok := n.(*ast.BinaryExpr); ok {
		if binOp.Op.String() == "+" {
			if isStringLiteral(binOp.X) && isStringLiteral(binOp.Y) {
				if xStr, ok := binOp.X.(*ast.BasicLit); ok {
					if strings.Contains(xStr.Value, "_") && len(xStr.Value) < 20 {
						pos := pv.ctx.FileSet.Position(binOp.Pos())
						*pv.violations = append(*pv.violations, Violation{
							RuleID:  "B8-no-manual-prefix",
							Pos:     pos,
							Subject: "manual concat",
							Message: fmt.Sprintf("manual ID prefix concatenation in %q", pv.pkg.PkgPath),
							FixHint: "Use core/pkg/ids.New(prefix) to generate all persistent IDs",
						})
					}
				}
			}
		}
	}

	return pv
}

func isStringLiteral(expr ast.Expr) bool {
	_, ok := expr.(*ast.BasicLit)
	return ok
}
