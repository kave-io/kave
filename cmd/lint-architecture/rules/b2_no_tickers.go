package rules

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/packages"
)

type B2NoTickers struct{}

func (B2NoTickers) ID() string { return "B2-no-tickers-in-core" }
func (B2NoTickers) Description() string {
	return "core/* must not use time.NewTicker or time.AfterFunc without explicit allow comment"
}

func (B2NoTickers) Check(ctx *Context) []Violation {
	var out []Violation

	for _, pkg := range ctx.Packages {
		if !strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/core/") {
			continue
		}

		for _, file := range pkg.Syntax {
			v := &tickerVisitor{
				pkg:        pkg,
				ctx:        ctx,
				allowlist:  ctx.Allowlist["B2-no-tickers-in-core"],
				violations: &out,
			}
			ast.Walk(v, file)
		}
	}

	return out
}

type tickerVisitor struct {
	pkg        *packages.Package
	ctx        *Context
	allowlist  []Allow
	violations *[]Violation
}

func (tv *tickerVisitor) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}
	if call, ok := n.(*ast.CallExpr); ok {
		var funcName string
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if x, ok := fn.X.(*ast.Ident); ok && x.Name == "time" {
				funcName = fn.Sel.Name
			}
		}

		if funcName == "NewTicker" || funcName == "AfterFunc" {
			pos := tv.ctx.FileSet.Position(call.Pos())
			filePath := pos.Filename
			isAllowed := false
			for _, allow := range tv.allowlist {
				if strings.Contains(filePath, allow.Path) {
					isAllowed = true
					break
				}
			}

			if !isAllowed {
				*tv.violations = append(*tv.violations, Violation{
					RuleID:  "B2-no-tickers-in-core",
					Pos:     pos,
					Subject: "time." + funcName,
					Message: fmt.Sprintf("core package %q uses time.%s which initiates scheduling", tv.pkg.PkgPath, funcName),
					FixHint: "Add // allow:B2 <reason> comment if intentional and update allowlist/tickers.txt",
				})
			}
		}
	}

	return tv
}
