# SQLite migrations

This directory now follows the same monotonic numbering used by the first-development migration history.

The earlier gap around `005` and `006` was removed by renumbering the later files so the sequence is continuous:

- `001_app`
- `002_price_snapshots`
- `003_price_book`
- `004_fx`
- `005_identity`
- `006_trace_fields`
- `007_budget_block_fields`
- `008_policy_casbin_document`

Rules going forward:

- Allocate the next unused number in sequence.
- Do not namespace numbers by topic.
- Keep migration filenames aligned with their execution order.
