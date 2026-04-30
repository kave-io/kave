package rules

import (
	"fmt"
	"strings"
)

type B5FrameworkLLMSeparation struct{}

func (B5FrameworkLLMSeparation) ID() string          { return "B5-framework-llm-separation" }
func (B5FrameworkLLMSeparation) Description() string { return "frameworks/* must not import llm/*" }

func (B5FrameworkLLMSeparation) Check(ctx *Context) []Violation {
	var out []Violation

	for _, pkg := range ctx.Packages {
		if !strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/core/connectors/inbound/frameworks") {
			continue
		}

		for _, imp := range pkg.Imports {
			if strings.HasPrefix(imp.PkgPath, "github.com/kave-io/kave/core/connectors/outbound/llm") {
				out = append(out, Violation{
					RuleID:  "B5-framework-llm-separation",
					Subject: imp.PkgPath,
					Message: fmt.Sprintf("framework package %q imports llm package %q", pkg.PkgPath, imp.PkgPath),
					FixHint: "Frameworks parse requests; LLM connectors talk to providers. Gateway holds both; they must not.",
				})
			}
		}
	}

	return out
}
