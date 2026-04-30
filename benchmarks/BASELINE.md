# Benchmark Baseline

Populate this table after running `make bench` on the dev host. CI compares
nightly results against these numbers; a regression >15% on any row fails.

```
Host: <CPU model>, <cores>, <RAM>, Linux <kernel>
Go:   <go version>
Date: YYYY-MM-DD
Commit: <sha>
```

| Benchmark | Sub | ns/op | B/op | allocs/op | Notes |
|---|---|---:|---:|---:|---|
| BenchmarkPipelineExecute | - |  |  |  | |
| BenchmarkGatewayForward_Buffered | body=1KB |  |  |  | |
| BenchmarkGatewayForward_Buffered | body=16KB |  |  |  | |
| BenchmarkGatewayForward_Buffered | body=256KB |  |  |  | |
| BenchmarkGatewayForward_Streaming | chunks=10 |  |  |  | |
| BenchmarkGatewayForward_Streaming | chunks=100 |  |  |  | |
| BenchmarkGatewayForward_Streaming | chunks=1000 |  |  |  | |
| BenchmarkSpanStore_Insert_DuckDB | single |  |  |  | |
| BenchmarkSpanStore_Insert_DuckDB | batch=100 |  |  |  | |
| BenchmarkSpanStore_Insert_DuckDB | batch=10000 |  |  |  | |
| BenchmarkSpanStore_Insert_Postgres | single |  |  |  | |
| BenchmarkSpanStore_Insert_Postgres | batch=100 |  |  |  | |
| BenchmarkSpanStore_Insert_Postgres | batch=10000 |  |  |  | |
| BenchmarkSpanStore_Query_WithFilter | duckdb |  |  |  | |
| BenchmarkSpanStore_Query_WithFilter | postgres |  |  |  | |
| BenchmarkTraceTree_Build_1k_Spans | n=1000 |  |  |  | |
| BenchmarkTraceTree_Build_1k_Spans | n=10000 |  |  |  | |
| BenchmarkTraceTree_Build_1k_Spans | n=100000 |  |  |  | |
| BenchmarkCostMeter_Compute | - |  |  |  | allocs must be 0 |
| BenchmarkSSEFanout_100Subscribers | subs=10 |  |  |  | |
| BenchmarkSSEFanout_100Subscribers | subs=100 |  |  |  | |
| BenchmarkSSEFanout_100Subscribers | subs=1000 |  |  |  | |
| BenchmarkMoneyAmount_AddMul | - |  |  |  | allocs must be 0 |
| BenchmarkAuthHash_Verify | correct |  |  |  | |
| BenchmarkAuthHash_Verify | wrong_first_byte |  |  |  | timing-safe |
| BenchmarkAuthHash_Verify | wrong_last_byte |  |  |  | timing-safe |

Companion file `BASELINE.txt` holds the raw `go test -bench` output for
`benchstat` comparisons.
