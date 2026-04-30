package rules

import (
	"fmt"
	"go/ast"
	"strings"
)

type B3BudgetCardinality struct{}

func (B3BudgetCardinality) ID() string          { return "B3-budget-cardinality" }
func (B3BudgetCardinality) Description() string { return "Only Agent may have BudgetCap field in v1" }

func (B3BudgetCardinality) Check(ctx *Context) []Violation {
	var out []Violation

	for _, pkg := range ctx.Packages {
		if !strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/core/model") {
			continue
		}

		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				if typeDecl, ok := decl.(*ast.GenDecl); ok && typeDecl.Tok.String() == "type" {
					for _, spec := range typeDecl.Specs {
						if typeSpec, ok := spec.(*ast.TypeSpec); ok {
							if typeSpec.Name.Name == "Agent" {
								continue
							}
							if structType, ok := typeSpec.Type.(*ast.StructType); ok {
								for _, field := range structType.Fields.List {
									if containsForbiddenBudgetField(field) {
										pos := ctx.FileSet.Position(field.Pos())
										out = append(out, Violation{
											RuleID:  "B3-budget-cardinality",
											Pos:     pos,
											Subject: typeSpec.Name.Name,
											Message: fmt.Sprintf("type %q has budget field; only Agent may have BudgetCap in v1", typeSpec.Name.Name),
											FixHint: "Move budget to Agent; post-v1 roadmap covers org/project/env budgets",
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

func containsForbiddenBudgetField(field *ast.Field) bool {
	for _, name := range field.Names {
		switch name.Name {
		case "BudgetCap", "BudgetLimit", "SpendLimit":
			return true
		}
	}
	return false
}
