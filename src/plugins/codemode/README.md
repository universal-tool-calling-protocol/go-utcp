# CodeMode UTCP – Go-Like DSL for Tool Orchestration

CodeMode UTCP is a system that enables LLMs to orchestrate Universal Tool Calling Protocol (UTCP) tools by generating and executing Go-like code snippets. It uses the Yaegi Go interpreter to safely evaluate user code while providing a structured interface to call external tools.

## Overview

CodeMode UTCP bridges an LLM's understanding of tool semantics with dynamic code execution. Instead of the LLM making sequential tool decisions, it generates Go-like code that chains tools together, handles tool outputs, and produces a final result—all validated and executed in a sandboxed environment.

### Key Components

- **orchestrator.go** – LLM-driven decision pipeline that determines which tools to call and generates code snippets
- **codemode.go** – Code execution engine using Yaegi, with helper functions injected for tool access

## How It Works

### 1. Tool Decision Pipeline

When `CallTool()` is invoked with a user prompt, the orchestrator executes four steps:

1. **Decide if tools are needed** – LLM determines whether UTCP tools are relevant
2. **Select appropriate tools** – LLM identifies which tool names match the intent
3. **Generate Go snippet** – LLM writes Go code using only the selected tools
4. **Execute and return** – CodeMode runs the snippet and returns the result

### 2. Code Generation

The LLM generates a Go snippet following strict rules:

- **Use only selected tool names** – No inventing or modifying tool identifiers
- **Use exact input/output schema keys** – Fields must match tool specifications exactly
- **Use provided helper functions** – `codemode.CallTool()`, `codemode.CallToolStream()`, etc.
- **No package or import declarations** – The system wraps the snippet automatically
- **Assign final result to `__out`** – The return value must be stored in this variable

### Example Generated Code

```go
// User query: "Get sum of 5 and 7, then multiply by 3"

r1, err := codemode.CallTool("math.add", map[string]any{
    "a": 5,
    "b": 7,
})
if err != nil { return err }

var sum any
if m, ok := r1.(map[string]any); ok {
    sum = m["result"]
}

r2, err := codemode.CallTool("math.multiply", map[string]any{
    "a": sum,
    "b": 3,
})

__out = map[string]any{
    "sum": sum,
    "product": r2,
}
```

## API Reference

### CodeModeUTCP

Main entry point for orchestrating tool calls.

#### NewCodeModeUTCP

```go
func NewCodeModeUTCP(
    client utcp.UtcpClientInterface,
    model interface { Generate(ctx context.Context, _, prompt string) (string, error) }
) *CodeModeUTCP
```

Creates a new CodeMode instance with a UTCP client and LLM model.

#### CallTool

```go
func (cm *CodeModeUTCP) CallTool(
    ctx context.Context,
    prompt string,
) (bool, any, error)
```

Orchestrates tool selection and execution. Returns a boolean indicating whether tools were used, the result, and any error.

#### Execute

```go
func (c *CodeModeUTCP) Execute(
    ctx context.Context,
    args CodeModeArgs,
) (CodeModeResult, error)
```

Directly executes a Go snippet with a specified timeout (in milliseconds).

### CodeModeArgs

```go
type CodeModeArgs struct {
    Code    string `json:"code"`
    Timeout int    `json:"timeout"`
}
```

- **Code** – Go-like DSL snippet
- **Timeout** – Execution timeout in milliseconds (default: 3000ms)

### CodeModeResult

```go
type CodeModeResult struct {
    Value  any    `json:"value"`
    Stdout string `json:"stdout"`
    Stderr string `json:"stderr"`
}
```

- **Value** – The result assigned to `__out`
- **Stdout** – Captured standard output
- **Stderr** – Captured standard error

## Helper Functions

Available within generated code snippets:

### CallTool

```go
result, err := codemode.CallTool(name string, args map[string]any) (any, error)
```

Calls a synchronous UTCP tool and returns its result.

### CallToolStream

```go
stream, err := codemode.CallToolStream(name string, args map[string]any) (*codeModeStream, error)

for {
    chunk, err := stream.Next()
    if err != nil { break }
    // process chunk
}
```

Calls a streaming tool and reads chunks in a loop.

### SearchTools

```go
tools, err := codemode.SearchTools(query string, limit int) ([]tools.Tool, error)
```

Searches available tools by query (useful for dynamic tool discovery).

### Sprintf / Errorf

```go
msg := codemode.Sprintf(format string, args ...any) string
err := codemode.Errorf(format string, args ...any) error
```

Standard Go formatting utilities.

## Code Normalization

CodeMode automatically normalizes user-provided code:

- **Package/Import stripping** – Removes `package` and `import` declarations
- **Walrus to assignment conversion** – Converts `__out :=` to `__out =`
- **Bare return fixing** – Replaces bare `return` statements with `return __out`
- **Automatic wrapping** – Wraps snippets into a complete Go program with proper structure
- **JSON to Go literal conversion** – Converts JSON objects to Go map literals

## Streaming Tools

When using streaming tools, mark the generated code with `"stream": true` in the response JSON:

```json
{
  "code": "stream, err := codemode.CallToolStream(\"api.fetch\", map[string]any{...}); ...",
  "stream": true
}
```

The orchestrator checks this flag to handle streaming contexts appropriately.

## Error Handling

Errors occur at multiple stages:

- **Tool decision errors** – LLM fails to determine if tools are needed
- **Tool selection errors** – LLM cannot identify appropriate tools
- **Code generation errors** – Generated snippet fails validation or syntax checks
- **Execution errors** – Runtime errors during snippet evaluation
- **Schema validation errors** – Generated code uses incorrect field names

All errors include context (stdout, stderr) to aid debugging.

## Configuration

### Environment Variables

- **utcp_search_tools_limit** – Maximum tools returned by `SearchTools` (default: 50)

## Validation Rules

Generated snippets must pass validation:

- **Must contain `__out` assignment** – Missing `__out` causes rejection
- **Must avoid invalid Go constructs** – E.g., bare map literals like `map[value:hello]`
- **Must use exact tool names** – No typos or modifications allowed
- **Must use exact schema keys** – Input/output field names must match specs exactly

## Tool Specs Reference

The orchestrator renders available tools with:

- **Name and Description**
- **Input field list** with types and required indicators
- **Full JSON input schema**
- **Output schema** showing the exact structure returned

This enables the LLM to make precise tool calls without guessing or inventing fields.

## Use Cases

- **Multi-step workflows** – Chain tools together with conditional logic
- **Data transformation** – Extract, process, and aggregate tool outputs
- **Error recovery** – Handle tool failures gracefully with branching logic
- **Dynamic tool discovery** – Use `SearchTools` to find relevant capabilities
- **Streaming aggregation** – Collect and process streaming results

## Example Workflow

```
User: "Search for Python tutorials and summarize the top 3 results"
       ↓
Orchestrator (decide) → Yes, tools needed
       ↓
Orchestrator (select) → ["search.web", "text.summarize"]
       ↓
Orchestrator (generate) → Code snippet that searches, extracts, and summarizes
       ↓
CodeMode (execute) → Runs snippet, returns structured results
       ↓
User receives aggregated summary
```

## Limitations

- Code snippets execute in a sandboxed Yaegi interpreter (no external process execution)
- Timeout prevents infinite loops (default 3s, configurable)
- No filesystem access unless explicitly provided via helpers
- Concurrent tool calls must be coordinated within the single-threaded Go snippet

## License

See LICENSE file in the repository.
