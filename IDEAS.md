# Kave Product Ideas

## Counterfactual Runs (Draft)

Date: 2026-04-01

### Naming

Recommended umbrella name: **Counterfactual Runs**

Other good options:
- **Shadow Runs**
- **Replay Branching**
- **Preview-to-Commit**
- **What-If Runs**

Proposed language:
- "Run this agent in a shadow branch."
- "Validate the predicted outcome."
- "Commit only if checks pass."

### Core idea

Let agents execute against a **shadow state** (not production state), then commit changes only after validation.

This is two capabilities that should ship together:
- **Shadow Execute + Commit Gate**: agent mutations are captured as a diff/patch against shadow data, never applied directly.
- **Replay + Drift Branching**: rerun from the exact historical data state, then fork from any step to test alternatives safely.

### Why this fits Kave

This extends the current Kave model (`Run`, `Action`, `Span`) and the four modules:
- `trace` records exact step-by-step execution and branch lineage.
- `auth` still enforces permissions before any write/commit.
- `validate` becomes the commit gate.
- `cost` compares historical vs branch spend before approval.

### Lifecycle

1. Start a run in `shadow` mode on a pinned snapshot.
2. Route reads to that snapshot and route writes to a shadow overlay.
3. Record every action normally (same trace tree model).
4. Compute a proposed mutation set (diff/patch).
5. Run validators and policies on proposed outcome.
6. If approved, apply patch atomically to real systems.
7. If rejected, discard the branch with full audit trail.

### Replay and drift

1. Pick an existing run and replay it against the original snapshot.
2. Step through actions exactly in order.
3. Fork from action `N` with changed prompt/policy/tool behavior.
4. Run the fork in shadow mode only.
5. Compare outcomes to original:
- final state diff
- validation pass/fail
- token/cost delta
- latency delta

### Minimal data model additions

- `Snapshot`: immutable pointer to data state used by a run.
- `Branch`: lineage metadata (`source_run_id`, `source_action_id`, `branch_reason`).
- `MutationPatch`: normalized proposed writes.
- `ValidationReport`: per-check results + final decision.
- `CommitRecord`: who approved, what was committed, when.

### CLI-first UX sketch

- `kave run --shadow ...`
- `kave replay <run_id>`
- `kave branch <run_id> --from-action <action_id> ...`
- `kave diff <branch_id>`
- `kave validate <branch_id>`
- `kave commit <branch_id>`

### Open questions

- Snapshot strategy per connector (DB MVCC snapshot, object version, cached API read model).
- Determinism level for LLM calls during replay (strict seed + stored outputs vs live model rerun).
- Commit semantics across multiple systems (single-system atomicity vs saga with compensation).
- Human approval gates vs fully automated policy-driven commit.
