# CodeMode UTCP — Expr runtime

CodeMode UTCP lets an LLM compose multiple UTCP tools into a single executable workflow. CodeMode now uses [Expr](https://expr-lang.org/) instead of Yaegi.

## Why Expr

Expr is a Go-centric expression language with static checking, a bytecode VM, bounded expression execution, and a small execution surface. CodeMode exposes only the `codemode` API to generated expressions, so generated code cannot import packages or access arbitrary Go APIs.

## How it works

1. The orchestrator ranks candidate UTCP tools.
2. The LLM receives an exact closed-world tool list and schemas.
3. The LLM generates Expr source.
4. CodeMode compiles the expression with Expr.
5. The expression calls only the exposed UTCP helpers.
6. The final Expr value becomes the CodeMode result.

## Expr syntax

Generated code is Expr, not Go. Use sequential expressions and `let` bindings:

```expr
let r1 = codemode.CallTool("math.add", {"a": 5, "b": 7});
let sum = codemode.Get(r1, "result");
let r2 = codemode.CallTool("math.multiply", {"value": sum, "factor": 3});
r2
```

The last expression is returned. There is no `__out`, `:=`, package declaration, import, type assertion, or Go loop.

## Runtime API

### `codemode.CallTool`

```text
codemode.CallTool("provider.tool", {"field": value})
```

Calls one UTCP tool. Tool errors propagate as Expr runtime errors.

### `codemode.CallToolStream`

```text
codemode.CallToolStream("provider.stream", {"input": "hello"})
```

Calls a streaming UTCP tool and returns the collected chunks as an array. This keeps streaming workflows compatible with Expr's expression-oriented execution model.

### `codemode.Get`

```text
codemode.Get(toolResult, "result")
```

Extracts a field from a `map[string]any` tool result and returns `nil` when the value is not a map or the key is absent.

## Chaining

Tool output can be passed directly into the next tool:

```expr
let first = codemode.CallTool("calculator.add", {"a": 2, "b": 3});
let value = codemode.Get(first, "result");
codemode.CallTool("calculator.multiply", {"value": value, "factor": 10})
```

This gives CodeMode a compact sequential workflow without repeatedly returning control to the LLM.

## Streaming

The orchestrator marks a generated plan as streaming when the expression contains `codemode.CallToolStream(...)`:

```json
{
  "tools": ["api.stream"],
  "code": "codemode.CallToolStream(\"api.stream\", {\"input\": \"hello\"})",
  "stream": true
}
```

## Safety boundary

The Expr environment exposes only the `codemode` object. The runtime does not expose Go imports, filesystem APIs, arbitrary reflection, or Yaegi's standard library loader. UTCP remains the authority for actual tool dispatch.

Generated tool names are checked against the exact candidate whitelist before execution.

## Timeout

`CodeModeArgs.Timeout` is expressed in milliseconds. The default direct-execution timeout is 30 seconds; the tool handler uses 3 seconds when no timeout is supplied. Remote tool calls receive the same cancellable context.

Expr itself does not support arbitrary infinite loops, which removes the primary reason the old Yaegi execution path needed to guard generated loops.

## API

```go
func NewCodeModeUTCP(
    client utcp.UtcpClientInterface,
    model interface {
        Generate(ctx context.Context, prompt string) (any, error)
    },
) *CodeModeUTCP

func (cm *CodeModeUTCP) CallTool(
    ctx context.Context,
    prompt string,
) (bool, any, error)

func (cm *CodeModeUTCP) Execute(
    ctx context.Context,
    args CodeModeArgs,
) (CodeModeResult, error)
```

## Environment

- `utcp_search_tools_limit` — maximum number of tools loaded into the CodeMode catalog; defaults to `50`.
- `UTCP_CODEMODE_CANDIDATE_LIMIT` — maximum candidate tools sent to the planner; defaults to `16`.

## Migration from Yaegi

The old Go-like CodeMode syntax is intentionally no longer the execution contract. Migrate generated programs to Expr using:

- `let x = ...` instead of Go variable declarations
- `{}` maps instead of `map[string]any{}`
- `;`-separated expressions instead of Go statements
- `codemode.Get(...)` instead of Go type assertions for common map results
- the final expression instead of `__out`
- `codemode.CallToolStream(...)` returning collected chunks instead of manual `Next()` loops

## Tests

The CodeMode test suite covers arithmetic and sequential expressions, synchronous tool calls, tool chaining, streaming, tool errors, timeout behavior, and exact tool-name extraction.
