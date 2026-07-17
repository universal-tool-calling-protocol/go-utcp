# From 448 KB to 13 KB per Search: Making go-utcp 3.2x Faster

When a UTCP client has a thousand tools to choose from, local tool selection should be almost invisible. The network call, model request, or tool itself will usually dominate end-to-end latency. The client should not spend hundreds of microseconds rebuilding and copying the same catalog before any real work begins.

That was not quite true in `go-utcp`.

On a catalog of 1,000 tools, a representative search took about 402 microseconds, allocated 448 KB, and performed 1,005 allocations. After reworking the repository and search paths, the same benchmark runs in about 127 microseconds, allocates 13 KB, and needs only seven allocations.

That is a **3.2x speedup**, with **97% less allocated memory** and **99.3% fewer allocations**.

The interesting part is not a single clever trick. The speedup came from changing what the hot path does—and, just as importantly, what it no longer does.

## The results

The following values are medians from three benchmark runs on an Apple M2 using Go 1.26.3 on Darwin/arm64. `B/op` and `allocs/op` are per operation.

| Path | Before | After | Improvement |
| --- | ---: | ---: | ---: |
| Search 1,000 tools, return top 16 | 402.5 us, 447.7 KB, 1,005 allocs | 126.5 us, 13.4 KB, 7 allocs | **3.18x faster**, 97.0% less memory |
| Rank 100 CodeMode tools, return top 16 | 44.9 us, 11.4 KB, 104 allocs | 36.3 us, 6.6 KB, 4 allocs | **1.24x faster**, 42.1% less memory |
| Cached CodeMode orchestration | 8.51 us, 12.3 KB, 69 allocs | 8.14 us, 11.1 KB, 66 allocs | 4.3% faster, 9.9% less memory |
| Cached dispatch among 1,000 tools | 11.6 ns, 0 B, 0 allocs | unchanged | Already near the floor |

The search benchmark uses ten providers with 100 tools each. Every tool has a description and several tags, and the strategy returns the 16 best matches for `search memory documents`.

## Where the time and memory went

The original search implementation was straightforward:

1. Ask the repository for every tool.
2. Copy the complete catalog into a new slice.
3. Lowercase and tokenize tool metadata for the current query.
4. Score every tool.
5. Keep and return the best matches.

That design is easy to reason about, but it makes a small-result query pay for the full catalog. A `Tool` is a relatively large struct containing schemas, tags, provider data, and handlers. Copying 1,000 of them accounted for most of the roughly 448 KB allocated by each search.

Metadata processing added another repeated cost. Tool names, tags, descriptions, and schema fields usually do not change between searches, yet the hot path normalized and scanned them again for every query.

The dispatch benchmark told a different story. Cached calls were already resolving among 1,000 tools in roughly 12 nanoseconds with no allocations. There was no reason to redesign that path. Profiling helped keep the work focused on the part that was actually expensive.

## Fix 1: iterate over a stable repository snapshot

The in-memory repository now exposes an optional iterator over a stable tool snapshot. Search strategies that understand this interface can inspect the catalog without allocating a full `[]Tool` copy first. Custom repository implementations remain compatible because the search strategy falls back to the original `GetTools` interface when the fast path is unavailable.

Snapshots are rebuilt only after a repository mutation. Readers can continue using an older immutable snapshot safely while a new one is published, so search does not need to hold the repository lock while scoring every tool.

This removes the largest per-query allocation without weakening the existing public repository contract.

## Fix 2: index metadata once, not once per query

Avoiding the catalog copy solved the memory spike, but it did not make scoring itself free. The next step was a revision-aware search index.

For each tool, the index stores the normalized tag values and the words extracted from its description. Queries still produce their own small word set, but scoring becomes a series of direct lookups against metadata that was prepared once.

The repository maintains an atomic catalog revision. Successful saves and removals increment it. The search strategy associates its index with that revision and rebuilds only when the visible tool catalog changes.

That gives repeated searches the fast path while preserving correctness after dynamic provider registration, replacement, or removal.

There is a deliberate tradeoff: the first search after a catalog change pays to build the index, and the index retains normalized metadata in memory. The benchmark measures steady-state search, which is the common case for clients that register providers during startup and issue many searches afterward.

## Fix 3: keep top-k bookkeeping small

For a query that asks for 16 results, moving full `Tool` values around during every insertion is unnecessary. The optimized selector keeps compact score records containing an index, score, and storage slot. Only tools that enter the bounded top-k set are copied into result storage.

This matters when many tools have similar scores: the algorithm can reorder small records while the larger tool values stay put.

The implementation also preallocates the bounded result storage. In the 1,000-tool benchmark, that helped bring the steady-state path down to seven allocations.

## Fix 4: remove avoidable CodeMode copies

CodeMode had two smaller sources of overhead.

First, its ranking function repeatedly lowercased descriptions, tags, and input field names. An allocation-free ASCII case-folding path now handles the common case, with a Unicode-preserving fallback for non-ASCII metadata. That reduced the 100-tool ranking benchmark from 104 allocations to four.

Second, the orchestration path defensively copied cached tool specifications even though it only reads them. CodeMode now uses an internal shared immutable snapshot, while exported cache accessors continue returning defensive copies. Public behavior remains safe, and the internal hot path avoids work it does not need.

## Correctness and concurrency

Performance changes around shared state are only useful if they remain correct under mutation and concurrency. The optimized implementation includes coverage for index invalidation after repository updates, and the verification suite included:

- 240 tests across 39 packages;
- 111 race-detector tests across the client and changed packages;
- a clean `go vet ./...` run.

Cancellation checks remain in catalog iteration and scoring loops. Tie-breaking remains deterministic within a snapshot, and custom repositories continue using the compatibility path.

## Reproducing the benchmarks

Run the focused search benchmark from the repository root:

```sh
go test ./src/tag \
  -run '^$' \
  -bench '^BenchmarkTagSearchStrategySearchTools$' \
  -benchmem \
  -count=3
```

Run the CodeMode ranking and orchestration benchmarks with:

```sh
go test ./src/plugins/codemode \
  -run '^$' \
  -bench '^(BenchmarkRankToolSpecs|BenchmarkCallTool_OneRoundTrip)$' \
  -benchmem \
  -count=3
```

And verify the already-fast dispatch path with:

```sh
go test . \
  -run '^$' \
  -bench '^BenchmarkCachedToolDispatch$' \
  -benchmem \
  -count=3
```

The comparison used `415304b` as the baseline and `ed0c9cf` (`v1.11.8`) as the optimized version.

These are local microbenchmarks. They intentionally exclude provider, network, model, and remote tool latency. The results describe the overhead controlled by the Go client itself; real end-to-end improvements depend on how much of an application's time is spent discovering and ranking tools.

## The broader lesson

The largest improvement did not come from making a loop slightly faster. It came from removing repeated work from the loop entirely.

The repository already knew when its catalog changed. Tool metadata was already effectively immutable between those changes. Once those facts became explicit—a stable snapshot plus a revision number—the search path could reuse prepared state instead of copying and normalizing the world on every query.

That pattern is broadly useful in read-heavy systems: publish immutable snapshots, invalidate them with a cheap generation counter, and keep the steady-state path focused on the request-specific work.

For `go-utcp`, the result is a search path that is faster, dramatically lighter on the allocator, and better suited to the large tool catalogs that UTCP clients increasingly need to manage.
