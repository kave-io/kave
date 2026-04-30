package rules

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/packages"
)

type B10HTTPAllowlist struct{}

func (B10HTTPAllowlist) ID() string          { return "B10-http-allowlist" }
func (B10HTTPAllowlist) Description() string { return "HTTP routes must match allowlist" }

func (B10HTTPAllowlist) Check(ctx *Context) []Violation {
	var out []Violation

	for _, pkg := range ctx.Packages {
		if !strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/server/") {
			continue
		}

		for _, file := range pkg.Syntax {
			v := &httpVisitor{
				pkg:        pkg,
				ctx:        ctx,
				violations: &out,
			}
			ast.Walk(v, file)
		}
	}

	return out
}

type httpVisitor struct {
	pkg        *packages.Package
	ctx        *Context
	violations *[]Violation
}

func (hv *httpVisitor) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}
	if call, ok := n.(*ast.CallExpr); ok {
		var routePath string
		isHTTPHandler := false

		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if sel.Sel.Name == "HandleFunc" {
				if len(call.Args) > 0 {
					if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						routePath = lit.Value[1 : len(lit.Value)-1]
						isHTTPHandler = true
					}
				}
			}
		}

		if isHTTPHandler && routePath != "" {
			if !matchesHTTPAllowlist(routePath, hv.ctx.Allowlist["B10-http-allowlist"]) {
				pos := hv.ctx.FileSet.Position(call.Pos())
				*hv.violations = append(*hv.violations, Violation{
					RuleID:  "B10-http-allowlist",
					Pos:     pos,
					Subject: routePath,
					Message: fmt.Sprintf("HTTP route %q not in allowlist", routePath),
					FixHint: "Add route to cmd/lint-architecture/allowlist/http.txt if intentional",
				})
			}
		}
	}

	return hv
}

func matchesHTTPAllowlist(route string, allowlist []Allow) bool {
	allowedPatterns := []string{
		"/health",
		"/v1/openai/",
		"/v1/anthropic/",
		"/v1/google/",
		"/frameworks/",
	}
	for _, pattern := range allowedPatterns {
		if matchesPattern(route, pattern) {
			return true
		}
	}
	for _, allow := range allowlist {
		if matchesPattern(route, allow.Path) {
			return true
		}
	}

	return false
}

func matchesPattern(route, pattern string) bool {
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(route, pattern) || route == strings.TrimSuffix(pattern, "/")
	}
	return route == pattern
}
