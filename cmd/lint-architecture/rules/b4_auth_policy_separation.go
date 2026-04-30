package rules

import (
	"fmt"
	"strings"
)

type B4NoAuthInPolicy struct{}

func (B4NoAuthInPolicy) ID() string          { return "B4-no-auth-in-policy" }
func (B4NoAuthInPolicy) Description() string { return "server/ops/policy/* must not import server/ops/auth/*" }

func (B4NoAuthInPolicy) Check(ctx *Context) []Violation {
	var out []Violation

	for _, pkg := range ctx.Packages {
		if !strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/server/ops/policy") {
			continue
		}

		for _, imp := range pkg.Imports {
			if strings.HasPrefix(imp.PkgPath, "github.com/kave-io/kave/server/ops/auth") {
				out = append(out, Violation{
					RuleID:  "B4-no-auth-in-policy",
					Subject: imp.PkgPath,
					Message: fmt.Sprintf("policy package %q imports auth package %q", pkg.PkgPath, imp.PkgPath),
					FixHint: "Policy reads identity from context; use authctx.Identity instead of importing auth package",
				})
			}
		}
	}

	return out
}

type B4NoPolicyInAuth struct{}

func (B4NoPolicyInAuth) ID() string          { return "B4-no-policy-in-auth" }
func (B4NoPolicyInAuth) Description() string { return "server/ops/auth/* must not import server/ops/policy/*" }

func (B4NoPolicyInAuth) Check(ctx *Context) []Violation {
	var out []Violation

	for _, pkg := range ctx.Packages {
		if !strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/server/ops/auth") {
			continue
		}

		for _, imp := range pkg.Imports {
			if strings.HasPrefix(imp.PkgPath, "github.com/kave-io/kave/server/ops/policy") {
				out = append(out, Violation{
					RuleID:  "B4-no-policy-in-auth",
					Subject: imp.PkgPath,
					Message: fmt.Sprintf("auth package %q imports policy package %q", pkg.PkgPath, imp.PkgPath),
					FixHint: "Auth produces identity; policy consumes it. They must be independently mockable.",
				})
			}
		}
	}

	return out
}
