package main

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kave-io/kave/cmd/lint-architecture/rules"
	"golang.org/x/tools/go/packages"
)

type LoadOptions struct {
	Root    string
	Only    string
	Verbose bool
}

func Run(opts LoadOptions) []rules.Violation {
	if opts.Root == "" {
		opts.Root = "."
	}
	fset := token.NewFileSet()
	pkgs, err := loadPackages(opts.Root, fset)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading packages: %v\n", err)
		return nil
	}

	if opts.Verbose {
		fmt.Printf("loaded %d packages\n", len(pkgs))
	}
	allowlist := loadAllowlist(opts.Root)
	ctx := &rules.Context{
		Packages:   pkgs,
		Allowlist:  allowlist,
		ModuleRoot: opts.Root,
		Verbose:    opts.Verbose,
		FileSet:    fset,
	}
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
	sort.Slice(allViolations, func(i, j int) bool {
		if allViolations[i].Pos.Filename != allViolations[j].Pos.Filename {
			return allViolations[i].Pos.Filename < allViolations[j].Pos.Filename
		}
		return allViolations[i].Pos.Line < allViolations[j].Pos.Line
	})

	return allViolations
}

func loadPackages(root string, fset *token.FileSet) ([]*packages.Package, error) {
	dirs := workspaceDirs(root)
	if len(dirs) == 0 {
		dirs = []string{root}
	}
	var all []*packages.Package
	for _, dir := range dirs {
		pkgs, err := packages.Load(
			&packages.Config{
				Mode: packages.NeedImports | packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
				Dir:  dir,
				Fset: fset,
			},
			"./...",
		)
		if err != nil {
			return nil, err
		}
		all = append(all, pkgs...)
	}
	return all, nil
}

func workspaceDirs(root string) []string {
	content, err := os.ReadFile(filepath.Join(root, "go.work"))
	if err != nil {
		return nil
	}
	var dirs []string
	seen := map[string]struct{}{}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, ",")
		line = strings.Trim(line, "\"")
		if !strings.HasPrefix(line, "./") && line != "." {
			continue
		}
		dir := filepath.Clean(filepath.Join(root, line))
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		dirs = append(dirs, dir)
	}
	return dirs
}

func loadAllowlist(root string) map[string][]rules.Allow {
	allowlist := make(map[string][]rules.Allow)
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

			// Strip optional HTTP method prefix (e.g. "POST /foo" -> "/foo", "* /foo" -> "/foo").
			if idx := strings.Index(path, " "); idx > 0 && !strings.HasPrefix(path, "/") {
				path = strings.TrimSpace(path[idx+1:])
			}

			allowlist[f.ruleID] = append(allowlist[f.ruleID], rules.Allow{
				Path:   path,
				Reason: reason,
			})
		}
	}

	return allowlist
}
func FormatViolation(v rules.Violation) string {
	return fmt.Sprintf(
		"%s:%d:%d: %s: %s — %s\n  fix: %s",
		v.Pos.Filename, v.Pos.Line, v.Pos.Column,
		v.RuleID, v.Message, v.Subject,
		v.FixHint,
	)
}
