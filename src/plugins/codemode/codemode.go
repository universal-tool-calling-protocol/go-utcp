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

type CodeModeArgs struct { Code string `json:"code"`; Timeout int `json:"timeout"` }
type CodeModeResult struct { Value any `json:"value"`; Stdout string `json:"stdout"`; Stderr string `json:"stderr"` }

type CodeModeUTCP struct {
	client utcp.UtcpClientInterface
	model interface { Generate(ctx context.Context, prompt string) (any, error) }
	executeFunc func(ctx context.Context, args CodeModeArgs) (CodeModeResult, error)
	cache *ToolCache
}

type exprModel struct { inner interface { Generate(ctx context.Context, prompt string) (any, error) } }

func (m exprModel) Generate(ctx context.Context, prompt string) (any, error) {
	const contract = `

============================================================
EXPR CODEMODE CONTRACT
============================================================

The CodeMode runtime is github.com/expr-lang/expr, NOT Yaegi and NOT a Go interpreter.
The generated "code" MUST be valid Expr v1.17 syntax.
Use sequential expressions separated by ';', let declarations, map literals,
array literals, normal function/member calls, and make the final expression the result.

The only runtime API is the codemode object:
- codemode.CallTool("EXACT_TOOL_NAME", {"field": value})
- codemode.CallToolStream("EXACT_TOOL_NAME", {"field": value})
- codemode.Get(value, "field")

CallTool and CallToolStream propagate errors as Expr runtime errors.
Do not emit Go package/import declarations, :=, var, Go type assertions,
Go loops, __out, return statements, err variables, stream.Next(), or helper APIs.
Do not use markdown fences.
`
	return m.inner.Generate(ctx, prompt+contract)
}

func NewCodeModeUTCP(client utcp.UtcpClientInterface, model interface { Generate(ctx context.Context, prompt string) (any, error) }) *CodeModeUTCP {
	if model != nil { model = exprModel{inner: model} }
	return &CodeModeUTCP{client: client, model: model, cache: NewToolCache()}
}

func (c *CodeModeUTCP) Tools() ([]tools.Tool, error) {
	return []tools.Tool{{
		Name: CodeModeToolName,
		Description: "Execute safe Expr expressions with access to UTCP tools",
		Tags: []string{"codemode", "expr", "utcp"},
		Inputs: tools.ToolInputOutputSchema{Type: "object", Properties: map[string]interface{}{
			"code": map[string]interface{}{"type": "string", "description": "Expr source code"},
			"timeout": map[string]interface{}{"type": "integer", "description": "Timeout in milliseconds"},
		}, Required: []string{"code"}, Title: "CodeModeArgs"},
		Outputs: tools.ToolInputOutputSchema{Type: "object", Properties: map[string]interface{}{
			"value": map[string]interface{}{"type": "string"}, "stdout": map[string]interface{}{"type": "string"}, "stderr": map[string]interface{}{"type": "string"},
		}, Title: "CodeModeResult"},
		Handler: c.createToolHandler(),
	}}, nil
}

type exprCodeModeAPI struct { client utcp.UtcpClientInterface; ctx context.Context }

func (api exprCodeModeAPI) CallTool(name string, args map[string]any) (any, error) {
	if api.client == nil { return nil, fmt.Errorf("codemode client is nil") }
	return api.client.CallTool(api.ctx, name, args)
}

func (api exprCodeModeAPI) CallToolStream(name string, args map[string]any) ([]any, error) {
	if api.client == nil { return nil, fmt.Errorf("codemode client is nil") }
	stream, err := api.client.CallToolStream(api.ctx, name, args)
	if err != nil { return nil, fmt.Errorf("CallToolStream failed: %w", err) }
	items := make([]any, 0)
	for { item, err := stream.Next(); if err != nil { return items, nil }; items = append(items, item) }
}

func (api exprCodeModeAPI) Get(value any, key string) any {
	if value == nil { return nil }
	if m, ok := value.(map[string]any); ok { return m[key] }
	return nil
}

func (c *CodeModeUTCP) prepareWrappedProgram(code string) (string, error) { return normalizeSnippet(code), nil }

func normalizeSnippet(code string) string {
	code = strings.TrimSpace(code)
	if strings.HasPrefix(code, "```") {
		lines := strings.Split(code, "\n")
		if len(lines) > 0 { lines = lines[1:] }
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" { lines = lines[:len(lines)-1] }
		code = strings.TrimSpace(strings.Join(lines, "\n"))
	}
	return code
}

func preprocessUserCode(code string) string { return normalizeSnippet(code) }
func convertOutWalrus(code string) string { return code }

func extractGeneratedToolNames(code string) []string {
	matches := generatedToolCallRE.FindAllStringSubmatch(code, -1)
	if len(matches) == 0 { return nil }
	names := make([]string, 0, len(matches)); seen := make(map[string]struct{}, len(matches))
	for _, match := range matches { if len(match) != 2 || match[1] == "" { continue }; if _, ok := seen[match[1]]; ok { continue }; seen[match[1]] = struct{}{}; names = append(names, match[1]) }
	return names
}

func (c *CodeModeUTCP) Execute(ctx context.Context, args CodeModeArgs) (CodeModeResult, error) {
	if c.executeFunc != nil { return c.executeFunc(ctx, args) }
	timeoutMs := args.Timeout; if timeoutMs <= 0 { timeoutMs = 30000 }
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond); defer cancel()
	code, err := c.prepareWrappedProgram(args.Code)
	if err != nil { return CodeModeResult{}, fmt.Errorf("failed to prepare Expr program: %w", err) }
	if strings.TrimSpace(code) == "" { return CodeModeResult{}, fmt.Errorf("empty Expr program") }
	env := map[string]any{"codemode": exprCodeModeAPI{client: c.client, ctx: ctx}}
	program, err := expr.Compile(code, expr.Env(env), expr.AsAny())
	if err != nil { return CodeModeResult{Stderr: err.Error()}, fmt.Errorf("Expr compilation failed: %w", err) }
	type runResult struct { value any; err error }
	done := make(chan runResult, 1)
	go func() { value, err := expr.Run(program, env); done <- runResult{value: value, err: err} }()
	select {
	case <-ctx.Done(): return CodeModeResult{Stderr: ctx.Err().Error()}, fmt.Errorf("execution timed out after %dms", timeoutMs)
	case result := <-done:
		if result.err != nil { return CodeModeResult{Stderr: result.err.Error()}, fmt.Errorf("Expr runtime error: %w", result.err) }
		return CodeModeResult{Value: result.value}, nil
	}
}

func (cm *CodeModeUTCP) createToolHandler() tools.ToolHandler {
	return func(ctx context.Context, inputs map[string]interface{}) (any, error) {
		args := CodeModeArgs{}
		if code, ok := inputs["code"].(string); ok { args.Code = code }
		if timeout, ok := inputs["timeout"].(float64); ok { args.Timeout = int(timeout) } else if timeout, ok := inputs["timeout"].(int); ok { args.Timeout = timeout }
		if args.Timeout <= 0 { args.Timeout = 3000 }
		result, err := cm.Execute(ctx, args)
		if err != nil { return nil, fmt.Errorf("error executing codemode expression: %w", err) }
		if result.Stderr != "" { return nil, fmt.Errorf("codemode expression produced an error: %s", result.Stderr) }
		return result.Value, nil
	}
}

func (cm *CodeModeUTCP) InvalidateToolSpecsCache() { if cm.cache != nil { cm.cache.InvalidateToolSpecs() } }
func (cm *CodeModeUTCP) InvalidateSelectionsCache() { if cm.cache != nil { cm.cache.InvalidateSelections() } }
func (cm *CodeModeUTCP) InvalidateAllCaches() { if cm.cache != nil { cm.cache.InvalidateAll() } }
func (cm *CodeModeUTCP) CacheStats() CacheStats { if cm.cache == nil { return CacheStats{} }; return cm.cache.Stats() }
func (cm *CodeModeUTCP) StartCacheCleanup(ctx context.Context, interval time.Duration) { if cm.cache != nil { cm.cache.StartCleanupRoutine(ctx, interval) } }
