package rules

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/packages"
)

type B2EgressChokepoint struct{}

func (B2EgressChokepoint) ID() string { return "B2-egress-chokepoint" }
func (B2EgressChokepoint) Description() string {
	return "HTTP client.Do() calls only in outbound connectors or gateway/transport.go"
}

func (B2EgressChokepoint) Check(ctx *Context) []Violation {
	var out []Violation

	for _, pkg := range ctx.Packages {
		if !strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/core/") &&
			!strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/server/") {
			continue
		}

		for _, file := range pkg.Syntax {
			v := &egressVisitor{
				pkg:        pkg,
				ctx:        ctx,
				violations: &out,
			}
			ast.Walk(v, file)
		}
	}

	return out
}

type egressVisitor struct {
	pkg        *packages.Package
	ctx        *Context
	violations *[]Violation
}

func (ev *egressVisitor) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}
	if call, ok := n.(*ast.CallExpr); ok {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Do" {
			pos := ev.ctx.FileSet.Position(call.Pos())
			filePath := pos.Filename
			allowedPaths := []string{
				"/core/connectors/outbound/",
				"/server/internal/gateway/transport.go",
			}

			isAllowed := false
			for _, path := range allowedPaths {
				if strings.Contains(filePath, path) {
					isAllowed = true
					break
				}
			}

			if !isAllowed {
				*ev.violations = append(*ev.violations, Violation{
					RuleID:  "B2-egress-chokepoint",
					Pos:     pos,
					Subject: "client.Do",
					Message: fmt.Sprintf("HTTP egress in %q outside chokepoint", ev.pkg.PkgPath),
					FixHint: "Move HTTP calls to core/connectors/outbound/* or server/internal/gateway/transport.go",
				})
			}
		}
	}

	return ev
}
