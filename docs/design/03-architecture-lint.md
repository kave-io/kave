# Design: Architecture Linter — Durable Enforcement of Boundaries

**Status:** Locked v1 design. The single mechanism that prevents the
http-bridge incident from recurring.

## Goals

The linter enforces the rules in `00-boundaries.md` such that:

1. **A new agent cannot violate a rule without seeing the violation.** No
   silent drift — any non-conforming code fails CI.
2. **The linter does not degrade.** Rules survive refactors. New rules
   can be added without rewriting old ones. Rules use the Go toolchain's
   own type/import data, not text grep over `.go` files.
3. **Overrides are auditable.** Every exception is in one allowlist file
   with a reason. The git diff on that file is the architecture review.
4. **Clear errors, fast feedback.** Violations point at file:line with
   the rule name and a suggested fix. `make lint-arch` runs in <10s.

## Non-goals

- Replacing `staticcheck`, `golangci-lint`, `go vet`. The architecture
  linter does **only** structural rules; bugs are caught by the
  general-purpose linters.
- Linting at LSP-time inside the editor. CI gating is the contract.
  Editor integration is nice-to-have post-v1.

## Why not regex?

Regex linters degrade. `\bgateway\b` matches a comment; `import.*uuid`
misses the dot-renamed import. The bridge incident happened in part
because nobody could write a quick grep for "is this code consistent with
the architecture" that they trusted.

This linter uses `golang.org/x/tools/go/packages` for type/import data
and `go/ast` for syntactic rules. Every rule operates on parsed Go, not
strings.

## Architecture

```
cmd/lint-architecture/
├── main.go                  // CLI: load packages, run rules, report
├── lint_test.go             // The actual CI gate — calls main as a Go test
├── rules/
│   ├── rule.go              // Rule interface, Violation type, Loader
│   ├── b1_layer_direction.go
│   ├── b2_no_schedulers.go
│   ├── b2_no_tickers.go
│   ├── b2_egress_chokepoint.go
│   ├── b3_budget_cardinality.go
│   ├── b4_auth_policy_separation.go
│   ├── b5_framework_llm_separation.go
│   ├── b6_no_cross_store_tx.go
│   ├── b6_single_span_writer.go
│   ├── b7_no_float_money.go
│   ├── b8_no_uuid.go
│   ├── b8_no_manual_prefix.go
│   ├── b9_no_time_in_models.go
│   ├── b9_time_now_justified.go
│   └── b10_http_allowlist.go
├── allowlist/
│   ├── http.txt             // route allowlist
│   ├── tickers.txt          // approved time.NewTicker / AfterFunc usages
│   └── overrides.txt        // // allow:Bx tracked entries
├── testdata/
│   └── <rule>/
│       ├── pass/...         // code that complies
│       └── fail/...         // code that violates; expected diagnostics
└── version.go               // RulesVersion bumped on rule changes
```

## Rule interface

```go
package rules

type Rule interface {
    ID() string                          // "B1-layer-direction"
    Description() string                 // one-sentence summary
    Check(ctx *Context) []Violation
}

type Context struct {
    Packages   []*packages.Package        // all loaded packages
    Allowlist  map[string][]Allow         // by rule ID
    FileSet    *token.FileSet
    ModuleRoot string
}

type Violation struct {
    RuleID  string
    Pos     token.Position                // file:line:col
    Subject string                        // identifier or import path
    Message string                        // human-readable explanation
    FixHint string                        // one-line suggestion
}

type Allow struct {
    Path   string                         // file or directory
    Reason string
}
```

## Allowlist format

`allowlist/<rule-shortname>.txt`:

```
# B2 — approved tickers
# Format: <package-or-file> ":" <reason>
core/fx/service.go : scheduled rate sync, observability not action
core/streamhub/heartbeat.go : keepalive only
```

Lines starting with `#` are comments. Blank lines ignored. Adding an entry
requires a PR; the diff is the review.

In-code overrides for ad-hoc cases use a `// allow:Bx <reason>` comment
adjacent (within 2 lines) to the offending statement. The linter
collects these into `allowlist/overrides.txt` automatically; `make
lint-arch-sync-overrides` rewrites the file. CI verifies the file is in
sync — if a new `// allow:` appears in code without an entry, fail.

This makes overrides discoverable in two places: at the call site (for
context) and centralized (for review).

## How a rule looks (concrete example)

`rules/b1_layer_direction.go`:

```go
type B1LayerDirection struct{}

func (B1LayerDirection) ID() string          { return "B1-layer-direction" }
func (B1LayerDirection) Description() string { return "proto must not import core; model must not import runtime" }

func (B1LayerDirection) Check(ctx *Context) []Violation {
    var out []Violation
    for _, pkg := range ctx.Packages {
        switch {
        case strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/proto/"):
            for _, imp := range pkg.Imports {
                if strings.Contains(imp.PkgPath, "/core/") {
                    out = append(out, Violation{
                        RuleID:  "B1-layer-direction",
                        Pos:     pkg.CompiledGoFiles[0],  // package-level
                        Subject: imp.PkgPath,
                        Message: fmt.Sprintf("proto package %q imports core package %q", pkg.PkgPath, imp.PkgPath),
                        FixHint: "proto/* must be self-contained; move shared types to proto/common/v1",
                    })
                }
            }
        case strings.HasPrefix(pkg.PkgPath, "github.com/kave-io/kave/core/model/"):
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
```

Every rule looks like this: load typed data, walk a few imports/decls,
emit violations. No regex.

## CLI surface

`cmd/lint-architecture/main.go`:

```
lint-architecture [flags]
  -only string      run only the named rule (e.g., "B1-layer-direction")
  -fix              currently a no-op; reserved for future autofixers
  -update-overrides regenerate allowlist/overrides.txt from // allow: comments
  -verbose          print which rules ran
```

Exit codes: `0` clean, `1` violations, `2` linter internal error.

## CI integration

The linter is a `go test`. `cmd/lint-architecture/lint_test.go`:

```go
func TestArchitectureLint(t *testing.T) {
    if testing.Short() {
        t.Skip("skipped in -short")
    }
    violations := lintarch.Run(lintarch.LoadOptions{Root: "../.."})
    if len(violations) > 0 {
        for _, v := range violations {
            t.Errorf("%s: %s — %s\n  fix: %s", v.RuleID, v.Pos, v.Message, v.FixHint)
        }
    }
}
```

The standard `make test-unit` target runs this alongside everything else.
A separate `make lint-arch` target invokes the binary directly for
faster iteration during rule development.

GitHub Actions: no special workflow needed — the existing `unit` job
catches it. Document this in the workflow YAML so reviewers know.

## Self-tests (the durability promise)

Every rule has fixtures:

```
testdata/B1-layer-direction/
├── pass/
│   └── pkg/...go        // code that complies
├── fail/
│   ├── pkg/...go        // code that violates
│   └── expected.txt     // expected violation lines
```

`rules/rule_test.go` walks `testdata/`, runs the named rule against
each `pass/` and `fail/` tree, and asserts:
- `pass/` produces zero violations.
- `fail/` produces exactly the violations in `expected.txt`.

When a rule is changed, the fixtures must be updated; the test makes
that mandatory.

## Versioning

`cmd/lint-architecture/version.go`:

```go
package lintarch

const RulesVersion = 7
```

Bumped on every rule add/change. The CI cache (if any) is keyed by
`RulesVersion`; bumping forces re-run. This is mostly defensive — the
linter is fast enough not to need caching, but the version still serves
as an explicit "the rules just changed" signal in PR diffs.

## What it catches today

Running a first pass against current code should flag at least:

- Two `ErrBudgetExceeded` definitions (B not yet a rule, but a duplicate-
  sentinel rule is easy to add — recommend deferring to v1.1 unless the
  cleanup PR introduces another duplicate).
- HTTP routes outside the allowlist (the four `/api/v1/fx/*` routes —
  resolved by the FX gRPC migration in `01-fx.md`).
- The `allowAnonymous` boolean (B-not-yet-a-rule, but worth a note).

## Adding a rule (developer workflow)

1. Append the rule to `00-boundaries.md` with `what / why / enforced by /
   override`.
2. Create `cmd/lint-architecture/rules/bN_my_rule.go` implementing the
   `Rule` interface.
3. Register it in `rules/registry.go`.
4. Add `testdata/bN-my-rule/pass/` and `testdata/bN-my-rule/fail/` with
   `expected.txt`.
5. Run `make lint-arch` locally; iterate until self-tests pass.
6. Bump `RulesVersion`.
7. Run `make lint-arch` against the whole repo; address every flagged
   violation (or add it to the allowlist with a reason — the PR review
   examines whether each entry deserves the override).

## Removing a rule

Same process in reverse, plus the note in `v1_decisions.md`. Removing a
rule is a serious step — it's saying "this boundary no longer matters."

## Extension points (post-v1)

Reserved seams for later, intentionally not built now:
- Autofixers (`-fix`) for mechanical violations like uuid imports.
- Editor integration via LSP / `gopls` extension.
- Per-file rule severity (warning vs error).

These are explicit non-goals for v1; the linter shipping with no fancy
features is an asset.

## Why this is durable

- Rules are ordinary Go code with tests. Refactoring `core/runtime` does
  not break the linter; the linter walks the new tree the same way.
- Fixtures encode intent. When somebody asks "why is this rule the way
  it is," the fail/ directory is the readable answer.
- Allowlist diffs are reviewable. No "// nolint:everything" sprinkled
  across the codebase — every override has a stated reason in one file.
- The linter runs as a test. There is no separate CI step to forget.
- `RulesVersion` makes rule changes explicit in PRs.

The thing that made the http-bridge incident expensive was the lack of a
shared, machine-checkable understanding of what the architecture *is*.
This linter is that understanding, written in Go, run on every PR.
