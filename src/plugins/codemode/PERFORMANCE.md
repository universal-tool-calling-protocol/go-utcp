# CodeMode Performance

## Latest results

Measured on an Apple M2 with `go test ./src/plugins/codemode -run '^$' -bench '^(BenchmarkInjectHelpers|BenchmarkExecuteSimple|BenchmarkToolSpecs_WithCache_Hit|BenchmarkCallTool_WithCache)$' -benchmem -count=5`.

| Benchmark | Before | Current | Change |
| --- | ---: | ---: | ---: |
| Helper injection | 957 us, 1.25 MB, 14,030 allocs | 6.0 us, 18.6 KB, 102 allocs | 99% faster |
| Simple execution | 1.14 ms, 1.40 MB, 15,329 allocs | 44 us, 59 KB, 747 allocs | 96% faster |
| Cached orchestration (mock model) | 32.7 us, 19.7 KB, 106 allocs | 12.2 us, 9.1 KB, 40 allocs | 63% faster |
| Cached tool-spec lookup | 3.8 us, 20 KB | 3.6 us, 20 KB | unchanged |

The orchestration benchmark uses an in-process mock model, so it does not represent provider/network latency. In production, the request path now has two sequential model calls—tool selection and code generation—instead of three. A cached selection skips the first of those calls altogether.

## What changed

1. Standard-library loading is now lazy. Most generated snippets only use CodeMode helpers, so Yaegi starts with no standard-library packages. `fmt` is loaded only for snippets that reference `fmt.`.
2. Normalization regexes are compiled once at package initialization instead of on every execution.
3. Tool selection uses a compact, cached name-and-description catalog rather than full input/output schemas.
4. Code generation receives full schemas only for the selected tools.
5. Selecting no tools replaces the separate tool-needed model decision, removing one model round trip.
6. Helpers propagate the execution context to UTCP calls, allowing compatible clients to stop work when CodeMode times out or is cancelled.

## Compatibility

Generated CodeMode snippets retain access to all documented helpers. Explicit uses of `fmt` remain supported. Other direct Yaegi standard-library imports are not part of the CodeMode DSL and are intentionally not loaded by default.
