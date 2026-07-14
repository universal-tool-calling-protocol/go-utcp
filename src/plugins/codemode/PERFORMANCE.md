# CodeMode Performance Report

- **Date:** 2026-07-14
- **Baseline:** `ae717a6d715b` (`fix codemode/orchestrator isValidSnippet`)
- **Current:** `78f597d45301` (`perf: optimize client repository and transports`)
- **Environment:** Apple M2, Go 1.26.3, Darwin/arm64, `-count=3`

## Summary

CodeMode execution is substantially faster after the optimization work:

- Simple snippets are **23.9x faster** and use **95.8% less memory**.
- A representative two-tool chain is **8.1x faster** and uses **92.0% less memory**.
- The cached orchestration path is **3.4x faster** with **one** model round trip instead of two after selection caching.

## Benchmark results

Values are medians of three benchmark processes. `B/op` and `allocs/op` are per operation.

| Path | Before | After | Change |
| --- | ---: | ---: | ---: |
| Helper injection | 910.8 us, 1.25 MB, 14,030 allocs | 6.2 us, 18.6 KB, 102 allocs | **146.8x faster**, 98.5% less memory |
| Simple execution | 1.026 ms, 1.40 MB, 15,328 allocs | 42.9 us, 59.1 KB, 747 allocs | **23.9x faster**, 95.8% less memory |
| Chained execution (two tools) | 1.108 ms, 1.46 MB, 16,143 allocs | 136.1 us, 116.9 KB, 1,562 allocs | **8.1x faster**, 92.0% less memory |
| Cached tool-spec lookup | 3.38 us, 20.0 KB, 1 alloc | 3.79 us, 20.0 KB, 1 alloc | 12.3% slower |
| Cached orchestration with mock model | 30.1 us, 19.7 KB, 106 allocs | 8.8 us, 12.3 KB, 69 allocs | **3.4x faster**, 37.4% less memory |

The small cached lookup regression is about 0.4 us and is outweighed by the execution and orchestration improvements.

## What was measured

- **Simple execution** runs an arithmetic CodeMode snippet through Yaegi.
- **Chained execution** calls local mock `math.add` and `math.multiply` tools in sequence, measuring interpreter, helper, and chaining overhead without network latency.
- **Cached orchestration** uses cached tool metadata and an in-process mock model. The baseline performed a decision plus code-generation model call after its selection cache was warm; the current path plans and generates in one call.

The orchestration benchmark intentionally excludes provider and network latency. Real model-backed requests should save the latency of one model round trip; remote tool-call latency will still dominate the chained-execution total when tools are network-bound.

## Reproducing the current measurements

Run the focused suite from the repository root:

```sh
go test ./src/plugins/codemode -run '^$' \
  -bench '^(BenchmarkInjectHelpers|BenchmarkExecuteSimple|BenchmarkExecuteChainedToolCalls|BenchmarkToolSpecs_WithCache_Hit|BenchmarkPlanAndGenerate_OneRoundTrip|BenchmarkCallTool_OneRoundTrip)$' \
  -benchmem -count=3
```

To reproduce the before values, run the matching benchmarks from a clean checkout of `ae717a6d715b`. `BenchmarkExecuteChainedToolCalls` was added with the optimization work; use the equivalent two-tool benchmark shown above when testing the baseline.
