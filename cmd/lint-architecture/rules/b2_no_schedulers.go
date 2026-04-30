package rules

import (
	"fmt"
	"strings"
)

type B2NoSchedulers struct{}

func (B2NoSchedulers) ID() string          { return "B2-no-schedulers-in-core" }
func (B2NoSchedulers) Description() string { return "core/* must not import cron schedulers" }

func (B2NoSchedulers) Check(ctx *Context) []Violation {
	var out []Violation
	forbiddenImports := []string{
		"github.com/robfig/cron",
		"github.com/jasonlvhit/gocron",
		"github.com/teambition/rrule-go",
	}

	for _, pkg := range ctx.Packages {
		if !strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/core/") {
			continue
		}

		for _, imp := range pkg.Imports {
			for _, forbidden := range forbiddenImports {
				if strings.HasPrefix(imp.PkgPath, forbidden) {
					out = append(out, Violation{
						RuleID:  "B2-no-schedulers-in-core",
						Subject: imp.PkgPath,
						Message: fmt.Sprintf("core package %q imports scheduler library %q", pkg.PkgPath, imp.PkgPath),
						FixHint: "Kave observes; it does not initiate. Remove the scheduler import and use request-driven lifecycle instead.",
					})
				}
			}
		}
	}

	return out
}
