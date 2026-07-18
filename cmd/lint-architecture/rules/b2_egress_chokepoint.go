package rules

import (
	"fmt"
	"go/ast"
	"go/types"
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
			if !isHTTPClientDo(ev.pkg, sel) {
				return ev
			}
			pos := ev.ctx.FileSet.Position(call.Pos())
			filePath := pos.Filename
			allowedPaths := []string{
				"/server/internal/gateway/transport.go",
				"/server/internal/v2/gateway/transport.go",
				"/core/fx/frankfurter.go",
				"/server/ops/auth/credresolve/vault.go",
				"/server/ops/fx/service.go",
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
					FixHint: "Move HTTP calls to server/internal/gateway/transport.go or add a documented B2 exception",
				})
			}
		}
	}

	return ev
}

func isHTTPClientDo(pkg *packages.Package, sel *ast.SelectorExpr) bool {
	if pkg == nil || pkg.TypesInfo == nil {
		return false
	}
	selection := pkg.TypesInfo.Selections[sel]
	if selection == nil || selection.Kind() != types.MethodVal {
		return false
	}
	fn, ok := selection.Obj().(*types.Func)
	if !ok {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	return types.TypeString(sig.Recv().Type(), nil) == "*net/http.Client"
}
