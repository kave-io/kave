package rules

import (
	"fmt"
	"go/token"
	"strings"
)

type B1LayerDirection struct{}

func (B1LayerDirection) ID() string { return "B1-layer-direction" }
func (B1LayerDirection) Description() string {
	return "proto must not import core; model must not import runtime"
}

func (B1LayerDirection) Check(ctx *Context) []Violation {
	var out []Violation

	for _, pkg := range ctx.Packages {
		if strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/proto/") {
			for _, imp := range pkg.Imports {
				if strings.Contains(imp.PkgPath, "/core/") {
					out = append(out, Violation{
						RuleID:  "B1-layer-direction",
						Pos:     token.Position{Filename: pkg.CompiledGoFiles[0]},
						Subject: imp.PkgPath,
						Message: fmt.Sprintf("proto package %q imports core package %q", pkg.PkgPath, imp.PkgPath),
						FixHint: "proto/* must be self-contained; move shared types to proto/common/v1",
					})
				}
			}
		}
		if strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/core/model/") {
			for _, imp := range pkg.Imports {
				if strings.HasPrefix(imp.PkgPath, "github.com/kave-io/kave/core/runtime/") {
					out = append(out, Violation{
						RuleID:  "B1-layer-direction",
						Subject: imp.PkgPath,
						Message: fmt.Sprintf("core/model package %q imports core/runtime %q", pkg.PkgPath, imp.PkgPath),
						FixHint: "model is storage-shaped; runtime types belong only in runtime and mappers",
					})
				}
			}
		}
	}

	return out
}
