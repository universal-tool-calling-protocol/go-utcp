# CodeMode Performance Notes

CodeMode now executes generated workflows with Expr's parser, type checker, optimizer, and bytecode VM instead of Yaegi.

The old benchmark numbers in this file measured Yaegi initialization and helper injection and are therefore not comparable to the new runtime. They were intentionally removed rather than presenting stale measurements as Expr results.

## What to benchmark

The useful Expr benchmarks are:

- expression compilation
- expression execution with a warm compiled program
- synchronous two-tool chaining
- streaming result collection
- cached tool catalog lookup
- planner/model round trips

Provider and network latency should be measured separately because UTCP tool calls can dominate end-to-end workflow latency.

## Runtime characteristics

Expr is designed as a safe expression evaluator with static checking and a bytecode VM. It does not expose arbitrary Go imports or Yaegi's standard library. Expr also does not provide arbitrary infinite loops, so the old interpreter-loop timeout benchmark is no longer representative.

Remote tool calls remain cancellable through the CodeMode execution context. CodeMode still applies an overall timeout around expression execution.
