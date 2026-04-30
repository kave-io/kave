package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kave-io/kave/cmd/lint-architecture/rules"
	"golang.org/x/tools/go/packages"
)

type LoadOptions struct {
	Root    string // repository root
	Only    string // only run this rule ID
	Verbose bool
}

// Run loads the workspace and runs all applicable rules.
func Run(opts LoadOptions) []rules.Violation {
	if opts.Root == "" {
		opts.Root = "."
	}

	// Load all packages in the module
	pkgs, err := packages.Load(
		&packages.Config{
			Mode: packages.NeedImports | packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
			Dir:  opts.Root,
		},
		"./...",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading packages: %v\n", err)
		return nil
	}

	if opts.Verbose {
		fmt.Printf("loaded %d packages\n", len(pkgs))
	}

	// Load allowlist
	allowlist := loadAllowlist(opts.Root)

	// Prepare context
	ctx := &rules.Context{
		Packages:   pkgs,
		Allowlist:  allowlist,
		ModuleRoot: opts.Root,
		Verbose:    opts.Verbose,
	}

	// Get token.FileSet from first package
	if len(pkgs) > 0 && pkgs[0].Fset != nil {
		ctx.FileSet = pkgs[0].Fset
	}

	// Run rules
	var allViolations []rules.Violation
	for _, rule := range rules.All() {
		if opts.Only != "" && rule.ID() != opts.Only {
			continue
		}

		if opts.Verbose {
			fmt.Printf("running rule %s: %s\n", rule.ID(), rule.Description())
		}

		violations := rule.Check(ctx)
		allViolations = append(allViolations, violations...)
	}

	// Sort by file and line
	sort.Slice(allViolations, func(i, j int) bool {
		if allViolations[i].Pos.Filename != allViolations[j].Pos.Filename {
			return allViolations[i].Pos.Filename < allViolations[j].Pos.Filename
		}
		return allViolations[i].Pos.Line < allViolations[j].Pos.Line
	})

	return allViolations
}

func loadAllowlist(root string) map[string][]rules.Allow {
	allowlist := make(map[string][]rules.Allow)

	// Load allowlist files
	allowlistDir := filepath.Join(root, "cmd/lint-architecture/allowlist")
	files := []struct {
		name   string
		ruleID string
	}{
		{"http.txt", "B10-http-allowlist"},
		{"tickers.txt", "B2-no-tickers-in-core"},
		{"overrides.txt", "overrides"},
	}

	for _, f := range files {
		content, err := os.ReadFile(filepath.Join(allowlistDir, f.name))
		if err != nil {
			continue
		}

		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}

			path := strings.TrimSpace(parts[0])
			reason := strings.TrimSpace(parts[1])

			allowlist[f.ruleID] = append(allowlist[f.ruleID], rules.Allow{
				Path:   path,
				Reason: reason,
			})
		}
	}

	return allowlist
}

// FormatViolation formats a violation for display.
func FormatViolation(v rules.Violation) string {
	return fmt.Sprintf(
		"%s:%d:%d: %s: %s — %s\n  fix: %s",
		v.Pos.Filename, v.Pos.Line, v.Pos.Column,
		v.RuleID, v.Message, v.Subject,
		v.FixHint,
	)
}
