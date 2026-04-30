package rules

import (
	"fmt"
	"go/ast"
	"regexp"
	"strings"
)

type B7NoFloatMoney struct{}

func (B7NoFloatMoney) ID() string { return "B7-no-float-money" }
func (B7NoFloatMoney) Description() string {
	return "No float32/float64 in cost/price/amount/spend/budget outside PriceBook"
}

func (B7NoFloatMoney) Check(ctx *Context) []Violation {
	var out []Violation

	moneyRegex := regexp.MustCompile(`(?i)(cost|price|amount|spend|budget)`)

	for _, pkg := range ctx.Packages {
		if !strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/") {
			continue
		}

		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				if typeDecl, ok := decl.(*ast.GenDecl); ok && typeDecl.Tok.String() == "type" {
					for _, spec := range typeDecl.Specs {
						if typeSpec, ok := spec.(*ast.TypeSpec); ok {
							if structType, ok := typeSpec.Type.(*ast.StructType); ok {
								for _, field := range structType.Fields.List {
									for _, name := range field.Names {
										if moneyRegex.MatchString(name.Name) {
											if isFloatType(field.Type) {
												if strings.Contains(pkg.PkgPath, "cost/service") {
													continue
												}

												pos := ctx.FileSet.Position(field.Pos())
												out = append(out, Violation{
													RuleID:  "B7-no-float-money",
													Pos:     pos,
													Subject: name.Name,
													Message: fmt.Sprintf("float field %q in %q; money must be int64", name.Name, typeSpec.Name.Name),
													FixHint: "Use int64 with unit (e.g., nano-USD, milli-Toman). Floats only in PriceBook.",
												})
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return out
}

func isFloatType(expr ast.Expr) bool {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name == "float32" || ident.Name == "float64"
	}
	return false
}
