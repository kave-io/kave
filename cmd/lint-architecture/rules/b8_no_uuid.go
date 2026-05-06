package rules

import (
	"fmt"
	"strings"
)

type B8NoUUID struct{}

func (B8NoUUID) ID() string { return "B8-no-uuid" }
func (B8NoUUID) Description() string {
	return "No direct uuid.NewString() usage; use core/pkg/ids instead"
}

func (B8NoUUID) Check(ctx *Context) []Violation {
	var out []Violation

	for _, pkg := range ctx.Packages {
		if !strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/") {
			continue
		}
		pkgName := pkg.PkgPath
		if strings.HasSuffix(pkgName, "_test") || strings.Contains(pkgName, "/ids") || strings.Contains(pkgName, "/internal/infra/paseto") {
			continue
		}

		for _, imp := range pkg.Imports {
			if strings.HasPrefix(imp.PkgPath, "github.com/google/uuid") {
				out = append(out, Violation{
					RuleID:  "B8-no-uuid",
					Subject: imp.PkgPath,
					Message: fmt.Sprintf("package %q imports google/uuid directly", pkg.PkgPath),
					FixHint: "Use core/pkg/ids.New(prefix) for prefixed ULIDs instead of uuid.NewString()",
				})
			}
		}
	}

	return out
}
