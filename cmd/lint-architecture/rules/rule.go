package rules

import (
	"go/token"

	"golang.org/x/tools/go/packages"
)

type Rule interface {
	ID() string
	Description() string
	Check(ctx *Context) []Violation
}
type Violation struct {
	RuleID  string
	Pos     token.Position
	Subject string
	Message string
	FixHint string
}
type Allow struct {
	Path   string
	Reason string
}
type Context struct {
	Packages   []*packages.Package
	Allowlist  map[string][]Allow
	FileSet    *token.FileSet
	ModuleRoot string
	Verbose    bool
}
type PackageContext struct {
	Package *packages.Package
	Context *Context
}

func (c *Context) MatchesAllowlist(ruleID, filePath string) bool {
	if allows, ok := c.Allowlist[ruleID]; ok {
		for _, allow := range allows {
			if filePath == allow.Path || contains(filePath, allow.Path) {
				return true
			}
		}
	}
	return false
}

func contains(haystack, needle string) bool {
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
