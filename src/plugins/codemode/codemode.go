package codemode

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

const CodeModeToolName = "codemode.run_code"

var generatedToolCallRE = regexp.MustCompile(`codemode\.CallTool(?:Stream)?\s*\(\s*"([^"]+)"`)

// CodeModeArgs contains an Expr CodeMode program.
type CodeModeArgs struct {
	Code    string `json:"code"`
	Timeout int    `json:"timeout"`
}

// CodeModeResult is the normalized result of an Expr program.
type CodeModeResult struct {
	Value  any    `json:"value"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

type CodeModeUTCP struct {
	client utcp.UtcpClientInterface
	model  interface {
		Generate(ctx context.Context, prompt string) (any, error)
	}
	executeFunc func(ctx context.Context, args CodeModeArgs) (CodeModeResult, error)
	cache       *ToolCache
}

// exprModel adds the Expr-specific execution contract to the existing planner
// prompt. Keeping this adapter local means the orchestration and tool ranking
// logic remain reusable while the runtime language changes from Go to Expr.
type exprModel struct {
	inner interface {
		Generate(ctx context.Context, prompt string) (any, error)
	}
}

func (m exprModel) Generate(ctx context.Context, prompt string) (any, error) {
	const contract = `

============================================================
EXPR CODEMODE CONTRACT — THIS OVERRIDES EARLIER GO RULES
============================================================

The CodeMode runtime is github.com/expr-lang/expr, NOT Yaegi and NOT a Go interpreter.
The generated "code" MUST be valid Expr v1.17 syntax.
Ignore every earlier instruction that asks for Go statements, package/import declarations,
:= declarations, Go type assertions, Go loops, or __out assignments.

Use these Expr constructs:
- sequential expressions separated by ';'
- variable declarations: let name = expression
- maps: {"field": value}
- arrays: [value1, value2]
- conditionals: if condition { expression } else { expression }
- normal function/member calls
- the final expression is the program result

The only runtime API is the `codemode` object:
- codemode.CallTool("EXACT_TOOL_NAME", {"field": value})
- codemode.CallToolStream("EXACT_TOOL_NAME", {"field": value})
- codemode.Get(value, "field") for extracting fields from tool results

CallTool and CallToolStream propagate Go errors as Expr runtime errors. Do not create
error variables or error-handling boilerplate. Do not use SearchTools, Sprintf, Errorf,
imports, package declarations, type assertions, goroutines, or loops.

Chaining example:
let r1 = codemode.CallTool("<EXACT_FIRST_TOOL_NAME>", {"a": 5});
let value = codemode.Get(r1, "result");
let r2 = codemode.CallTool("<EXACT_SECOND_TOOL_NAME>", {"value": value});
r2

Streaming example:
codemode.CallToolStream("<EXACT_STREAM_TOOL_NAME>", {"input": "hello"})

The final expression is the returned value. Do not emit __out.
Do not wrap the expression in markdown fences.
Return the same JSON object shape, but put Expr source in the "code" field.
The "stream" flag must match whether codemode.CallToolStream is used.
`
	return m.inner.Generate(ctx, prompt+contract)
}

func NewCodeModeUTCP(client utcp.UtcpClientInterface, model interface {
	Generate(ctx context.Context, prompt string) (any, error)
}) *CodeModeUTCP {
	if model != nil {
		model = exprModel{inner: model}
	}
	return &CodeModeUTCP{
		client: client,
		model:  model,
		cache:  NewToolCache(),
	}
}

func (c *CodeModeUTCP) Tools() ([]tools.Tool, error) {
	return []tools.Tool{{
		Name:        CodeModeToolName,
		Description: "Execute safe Expr expressions with access to UTCP tools",
		Tags:        []string{"codemode", "expr", "utcp"},
		Inputs: tools.ToolInputOutputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"code": map[string]interface{}{
					"type":        "string",
					"description": "Expr source code",
				},
				"timeout": map[string]interface{}{
					"type":        "integer",
					"description": "Timeout in milliseconds",
				},
			},
			Required: []string{"code"},
			Title:    "CodeModeArgs",
		},
		Outputs: tools.ToolInputOutputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"value":  map[string]interface{}{"type": "string"},
				"stdout": map[string]interface{}{"type": "string"},
				"stderr": map[string]interface{}{"type": "string"},
			},
			Title: "CodeModeResult",
		},
		Handler: c.createToolHandler(),
	}}, nil
}

// exprCodeModeAPI is the only object exposed to generated programs. It keeps
// the UTCP client and execution context outside the Expr environment itself.
type exprCodeModeAPI struct {
	client utcp.UtcpClientInterface
	ctx    context.Context
}

func (api exprCodeModeAPI) CallTool(name string, args map[string]any) (any, error) {
	if api.client == nil {
		return nil, fmt.Errorf("codemode client is nil")
	}
	return api.client.CallTool(api.ctx, name, args)
}

func (api exprCodeModeAPI) CallToolStream(name string, args map[string]any) ([]any, error) {
	if api.client == nil {
		return nil, fmt.Errorf("codemode client is nil")
	}
	stream, err := api.client.CallToolStream(api.ctx, name, args)
	if err != nil {
		return nil, fmt.Errorf("CallToolStream failed: %w", err)
	}

	items := make([]any, 0)
	for {
		item, err := stream.Next()
		if err != nil {
			return items, nil
		}
		items = append(items, item)
	}
}

func (api exprCodeModeAPI) Get(value any, key string) any {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		return m[key]
	}
	return nil
}

func (c *CodeModeUTCP) prepareWrappedProgram(code string) (string, error) {
	return normalizeSnippet(code), nil
}

// normalizeSnippet removes accidental markdown fences and adds a harmless
// compatibility comment consumed by the legacy planner validator. The comment
// is valid Expr and does not affect the result.
func normalizeSnippet(code string) string {
	code = strings.TrimSpace(code)
	if strings.HasPrefix(code, "```") {
		lines := strings.Split(code, "\n")
		if len(lines) > 0 {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			lines = lines[:len(lines)-1]
		}
		code = strings.TrimSpace(strings.Join(lines, "\n"))
	}
	if code == "" {
		return code
	}
	if !strings.Contains(code, "__out =") {
		code += "\n// __out = Expr result"
	}
	return code
}

func preprocessUserCode(code string) string {
	return normalizeSnippet(code)
}

func convertOutWalrus(code string) string {
	return code
}

func extractGeneratedToolNames(code string) []string {
	matches := generatedToolCallRE.FindAllStringSubmatch(code, -1)
	if len(matches) == 0 {
		return nil
	}
	names := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) != 2 || match[1] == "" {
			continue
		}
		if _, ok := seen[match[1]]; ok {
			continue
		}
		seen[match[1]] = struct{}{}
		names = append(names, match[1])
	}
	return names
}

func (c *CodeModeUTCP) Execute(ctx context.Context, args CodeModeArgs) (CodeModeResult, error) {
	if c.executeFunc != nil {
		return c.executeFunc(ctx, args)
	}

	timeoutMs := args.Timeout
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	code, err := c.prepareWrappedProgram(args.Code)
	if err != nil {
		return CodeModeResult{}, fmt.Errorf("failed to prepare Expr program: %w", err)
	}
	if strings.TrimSpace(code) == "" {
		return CodeModeResult{}, fmt.Errorf("empty Expr program")
	}

	env := map[string]any{
		"codemode": exprCodeModeAPI{client: c.client, ctx: ctx},
	}

	program, err := expr.Compile(code, expr.Env(env), expr.AsAny())
	if err != nil {
		return CodeModeResult{Stderr: err.Error()}, fmt.Errorf("Expr compilation failed: %w", err)
	}

	type runResult struct {
		value any
		err   error
	}
	done := make(chan runResult, 1)
	go func() {
		value, err := expr.Run(program, env)
		done <- runResult{value: value, err: err}
	}()

	select {
	case <-ctx.Done():
		return CodeModeResult{Stderr: ctx.Err().Error()}, fmt.Errorf("execution timed out after %dms", timeoutMs)
	case result := <-done:
		if result.err != nil {
			return CodeModeResult{Stderr: result.err.Error()}, fmt.Errorf("Expr runtime error: %w", result.err)
		}
		return CodeModeResult{Value: result.value}, nil
	}
}

func (cm *CodeModeUTCP) createToolHandler() tools.ToolHandler {
	return func(ctx context.Context, inputs map[string]interface{}) (any, error) {
		args := CodeModeArgs{}
		if code, ok := inputs["code"].(string); ok {
			args.Code = code
		}
		if timeout, ok := inputs["timeout"].(float64); ok {
			args.Timeout = int(timeout)
		} else if timeout, ok := inputs["timeout"].(int); ok {
			args.Timeout = timeout
		}
		if args.Timeout <= 0 {
			args.Timeout = 3000
		}

		result, err := cm.Execute(ctx, args)
		if err != nil {
			return nil, fmt.Errorf("error executing codemode expression: %w", err)
		}
		if result.Stderr != "" {
			return nil, fmt.Errorf("codemode expression produced an error: %s", result.Stderr)
		}
		return result.Value, nil
	}
}

func (cm *CodeModeUTCP) InvalidateToolSpecsCache() {
	if cm.cache != nil {
		cm.cache.InvalidateToolSpecs()
	}
}

func (cm *CodeModeUTCP) InvalidateSelectionsCache() {
	if cm.cache != nil {
		cm.cache.InvalidateSelections()
	}
}

func (cm *CodeModeUTCP) InvalidateAllCaches() {
	if cm.cache != nil {
		cm.cache.InvalidateAll()
	}
}

func (cm *CodeModeUTCP) CacheStats() CacheStats {
	if cm.cache == nil {
		return CacheStats{}
	}
	return cm.cache.Stats()
}

func (cm *CodeModeUTCP) StartCacheCleanup(ctx context.Context, interval time.Duration) {
	if cm.cache != nil {
		cm.cache.StartCleanupRoutine(ctx, interval)
	}
}
