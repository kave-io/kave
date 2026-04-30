package rules

import (
	"go/token"

	"golang.org/x/tools/go/packages"
)

// Rule is the interface all linter rules implement.
type Rule interface {
	ID() string
	Description() string
	Check(ctx *Context) []Violation
}

// Violation represents a single boundary violation.
type Violation struct {
	RuleID  string         // e.g. "B1-layer-direction"
	Pos     token.Position // file:line:col
	Subject string         // identifier or import path
	Message string         // human-readable explanation
	FixHint string         // suggested fix
}

// Allow represents an allowlist entry for a rule.
type Allow struct {
	Path   string // file or directory pattern
	Reason string
}

// Context provides rules with access to packages and allowlist data.
type Context struct {
	Packages    []*packages.Package // all loaded packages in the workspace
	Allowlist   map[string][]Allow  // by rule ID
	FileSet     *token.FileSet
	ModuleRoot  string
	Verbose     bool
}

// PackageContext is a helper for rules that need to examine a specific package.
type PackageContext struct {
	Package *packages.Package
	Context *Context
}

// MatchesAllowlist checks if a file/path is in the allowlist for the given rule.
func (c *Context) MatchesAllowlist(ruleID, filePath string) bool {
	if allows, ok := c.Allowlist[ruleID]; ok {
		for _, allow := range allows {
			// Simple substring match or exact match
			if filePath == allow.Path || contains(filePath, allow.Path) {
				return true
			}
		}
	}
	return false
}

func contains(haystack, needle string) bool {
	// Simple check: if the path contains the allowed pattern
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
