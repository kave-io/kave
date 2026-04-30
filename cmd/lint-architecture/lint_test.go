package main

import (
	"testing"
)

// TestArchitectureLint is the CI gate for architectural compliance.
func TestArchitectureLint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in -short")
	}

	violations := Run(LoadOptions{Root: "../.."})
	if len(violations) > 0 {
		for _, v := range violations {
			t.Errorf("%s: %s — %s\n  fix: %s", v.RuleID, v.Pos, v.Message, v.FixHint)
		}
		t.FailNow()
	}
}
