package rules

import (
	"fmt"
	"go/ast"
	"strings"
)

type B9NoTimeInModels struct{}

func (B9NoTimeInModels) ID() string { return "B9-no-time-in-models" }
func (B9NoTimeInModels) Description() string {
	return "core/model/* and proto/* must use int64 unix-ms, not time.Time"
}

func (B9NoTimeInModels) Check(ctx *Context) []Violation {
	var out []Violation

	for _, pkg := range ctx.Packages {
		if !strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/core/model") &&
			!strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/proto/") {
			continue
		}

		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				if typeDecl, ok := decl.(*ast.GenDecl); ok && typeDecl.Tok.String() == "type" {
					for _, spec := range typeDecl.Specs {
						if typeSpec, ok := spec.(*ast.TypeSpec); ok {
							if structType, ok := typeSpec.Type.(*ast.StructType); ok {
								for _, field := range structType.Fields.List {
									if isTimeType(field.Type) {
										pos := ctx.FileSet.Position(field.Pos())
										out = append(out, Violation{
											RuleID:  "B9-no-time-in-models",
											Pos:     pos,
											Subject: "time.Time",
											Message: fmt.Sprintf("model %q has time.Time field; use int64 unix-ms", typeSpec.Name.Name),
											FixHint: "Use int64 for unix-millisecond timestamps; time.Time only at edges (parsing, formatting)",
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

	return out
}

func isTimeType(expr ast.Expr) bool {
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		if x, ok := sel.X.(*ast.Ident); ok {
			return x.Name == "time" && sel.Sel.Name == "Time"
		}
	}
	return false
}
