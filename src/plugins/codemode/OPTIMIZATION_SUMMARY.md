# CodeMode Runtime Migration Summary

The CodeMode runtime on this branch replaces Yaegi with Expr.

## Runtime changes

- Removed the Yaegi interpreter and standard-library injection path.
- Added `github.com/expr-lang/expr` v1.17.8.
- Generated programs are now Expr expressions rather than Go-like snippets.
- Tool access is exposed through a single `codemode` API object.
- Tool errors propagate through Expr's normal `(value, error)` function semantics.
- Streaming tools are collected into an Expr array by `codemode.CallToolStream`.
- Tool-result map fields can be extracted with `codemode.Get`.
- The existing tool whitelist and orchestration cache remain unchanged.

## Expr safety model

Expr provides parsing, static checking, optimization, and bytecode execution without exposing arbitrary Go imports or Yaegi's standard library. CodeMode therefore has a smaller execution surface than the previous interpreter-based implementation.

Expr programs also do not expose arbitrary infinite loops. Remote UTCP calls still receive a cancellable context and CodeMode maintains an overall execution timeout.

## Workflow shape

A generated multi-tool workflow is now compact:

```expr
let first = codemode.CallTool("math.add", {"a": 2, "b": 3});
let value = codemode.Get(first, "result");
codemode.CallTool("math.multiply", {"value": value, "factor": 10})
```

The last expression is the workflow result, eliminating the old `__out` plumbing.

## Benchmark follow-up

The old benchmark suite measured Yaegi interpreter initialization and helper injection and was removed. New benchmarks should measure Expr compilation, warm execution, chained tool execution, streaming collection, and cached orchestration independently.
