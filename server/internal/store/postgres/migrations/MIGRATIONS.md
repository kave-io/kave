# Postgres migrations

This directory is in the corrected monotonic order used by the first-development migration history.

The `006` collision from the audit plan was resolved by renumbering the later migrations so filenames now advance strictly upward:

- `001_kave_core`
- `002_auth`
- `003_price_snapshots`
- `004_price_book`
- `005_cost_model_gaps`
- `006_budgets`
- `007_fx`
- `008_identity`
- `009_trace_fields`
- `010_budget_block_fields`
- `011_policy_casbin_document`
- `012_run_provenance`

Rules going forward:

- Allocate the next unused number in sequence.
- Do not namespace numbers by topic.
- Keep `*.up.sql` and `*.down.sql` pairs together under the same number.
