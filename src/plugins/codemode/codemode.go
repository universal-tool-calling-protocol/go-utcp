// path: codemode/codemode_utcp.go
package codemode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

//
// ───────────────────────────────────────────────────────────
//   UTCP CodeMode – using Yaegi Go Interpreter
// ───────────────────────────────────────────────────────────
//

const CodeModeToolName = "codemode.run_code"

type CodeModeArgs struct {
	Code    string `json:"code"`
	Timeout int    `json:"timeout"`
}

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
	// For testing purposes, to mock the Execute method.
	executeFunc func(ctx context.Context, args CodeModeArgs) (CodeModeResult, error)
}

func NewCodeModeUTCP(client utcp.UtcpClientInterface, model interface {
	Generate(ctx context.Context, prompt string) (any, error)
}) *CodeModeUTCP {
	return &CodeModeUTCP{client: client, model: model}
}

//
// ───────────────────────────────────────────────────────────
//   Register UTCP Tool
// ───────────────────────────────────────────────────────────
//

func (c *CodeModeUTCP) Tools() ([]tools.Tool, error) {
	return []tools.Tool{
		{
			Name:        CodeModeToolName,
			Description: "Execute Go-like DSL with access to UTCP tools (AST-based)",
			Tags:        []string{"codemode", "go", "utcp"},
			Inputs: tools.ToolInputOutputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"code": map[string]interface{}{
						"type":        "string",
						"description": "Go-like DSL code snippet",
					},
					"timeout": map[string]interface{}{
						"type":        "integer",
						"description": "Timeout in ms",
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
		},
	}, nil
}

//
// ───────────────────────────────────────────────────────────
//   EXECUTE DSL USING AST
// ───────────────────────────────────────────────────────────
//

func newInterpreter() (*interp.Interpreter, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer

	i := interp.New(interp.Options{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	return i, &stdout, &stderr
}

func (c *CodeModeUTCP) prepareWrappedProgram(code string) (string, error) {
	processed := preprocessUserCode(code)
	clean := normalizeSnippet(processed)
	return wrapIntoProgram(clean), nil
}

func preprocessUserCode(code string) string {
	trim := strings.TrimSpace(code)

	if strings.HasPrefix(trim, "{") && strings.HasSuffix(trim, "}") {
		return "__out = " + jsonToGoLiteral(trim)
	}

	code = stripPackageAndImports(code)
	code = convertOutWalrus(code) // This now handles all __out assignment conversions
	code = fixBareReturn(code)    // 👈 ADD THIS LINE
	code = ensureOutAssigned(code)
	return code
}

func jsonToGoLiteral(s string) string {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s // fallback to raw
	}
	return toGoLiteral(v)
}

func toGoLiteral(v any) string {
	switch val := v.(type) {

	case map[string]any:
		parts := make([]string, 0, len(val))
		for k, v2 := range val {
			parts = append(parts,
				fmt.Sprintf("%q: %s", k, toGoLiteral(v2)))
		}
		sort.Strings(parts)
		if len(parts) > 0 {
			// Add a trailing comma for multi-line safety
			return fmt.Sprintf("map[string]any{%s,}", strings.Join(parts, ", "))
		}
		return "map[string]any{}"

	case []any:
		items := make([]string, len(val))
		for i := range val {
			items[i] = toGoLiteral(val[i])
		}
		return fmt.Sprintf("[]any{%s}", strings.Join(items, ", "))

	case string:
		return fmt.Sprintf("%q", val)

	case float64, bool:
		return fmt.Sprintf("%v", val)

	case nil:
		return "nil"
	}

	return fmt.Sprintf("%#v", v)
}

func fixBareReturn(code string) string {
	// replace any `return` followed by end-of-line or `}` with `return __out`
	re := regexp.MustCompile(`(?m)^\s*return\s*$`)
	return re.ReplaceAllString(code, "return __out")
}

func convertOutWalrus(code string) string {
	// This regex finds `__out :=` and replaces it with `__out = `
	// It handles cases where __out is the only variable or the first of several.
	// e.g., `__out := ...` -> `__out = ...`
	// e.g., `__out, err := ...` -> `__out, err = ...` (which is incorrect if err is new, but better than a syntax error)
	// A more robust solution would be a proper Go parser, but this regex is a significant improvement.
	re := regexp.MustCompile(`__out\s*:=`)
	return re.ReplaceAllString(code, "__out = ")
}
func stripPackageAndImports(code string) string {
	// Remove package declaration
	rePackage := regexp.MustCompile(`(?m)^\s*package\s+\w+\s*$`)
	code = rePackage.ReplaceAllString(code, "")

	// Remove import declarations (single line and multi-line)
	reImportSingle := regexp.MustCompile(`(?m)^\s*import\s+(".*"|\w+\s+".*"|\(.*\))\s*$`)
	code = reImportSingle.ReplaceAllString(code, "")
	reImportMulti := regexp.MustCompile(`(?s)^\s*import\s*\((.*?)\)\s*$`)
	code = reImportMulti.ReplaceAllString(code, "")
	return code
}

func ensureOutAssigned(code string) string {
	if !strings.Contains(code, "__out") {
		trim := strings.TrimSpace(code)
		return "__out = " + trim
	}
	return code
}

// injectHelpers makes UTCP client functions available to the Yaegi interpreter.
type codeModeStream struct {
	next func() (any, error)
}

func wrapIntoProgram(clean string) string {
	return fmt.Sprintf(`package main

import (
	"context/context"
	codemode "codemode_helpers/codemode_helpers"
)

func run() any {
    var __out any

    // ----- BEGIN USER CODE -----
%s
    // ----- END USER CODE -----

    return __out
}
`, indent(clean, "    "))
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func (c *CodeModeUTCP) Execute(ctx context.Context, args CodeModeArgs) (CodeModeResult, error) {
	// Allow mocking for tests
	if c.executeFunc != nil {
		return c.executeFunc(ctx, args)
	}

	// 1. Enforce Timeout via Context
	// Convert integer ms to Duration. Default to 30s if invalid.
	timeoutMs := args.Timeout
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	i, stdout, stderr := newInterpreter()

	if err := injectHelpers(i, c.client); err != nil {
		return CodeModeResult{}, fmt.Errorf("failed to inject helpers: %w", err)
	}

	wrapped, err := c.prepareWrappedProgram(args.Code)
	if err != nil {
		return CodeModeResult{}, fmt.Errorf("failed to prepare program: %w", err)
	}

	// 2. Structure for async result handling
	type evalResult struct {
		val reflect.Value
		err error
	}
	done := make(chan evalResult, 1)

	// 3. Run Eval in a Goroutine
	go func() {
		// Safety: recover from internal interpreter panics
		defer func() {
			if r := recover(); r != nil {
				done <- evalResult{err: fmt.Errorf("interpreter panic: %v", r)}
			}
		}()

		// Phase A: Compilation & Definition
		if _, err := i.Eval(wrapped); err != nil {
			done <- evalResult{err: fmt.Errorf("compilation failed: %w", err)}
			return
		}

		// Phase B: Execution of the wrapped runner
		v, err := i.Eval(`main.run()`)
		done <- evalResult{val: v, err: err}
	}()

	// 4. Wait for Completion or Timeout
	select {
	case <-ctx.Done():
		return CodeModeResult{
			Stdout: stdout.String(),
			Stderr: stderr.String(),
		}, fmt.Errorf("execution timed out after %dms", timeoutMs)

	case res := <-done:
		if res.err != nil {
			return CodeModeResult{
				Stdout: stdout.String(),
				Stderr: stderr.String(),
			}, fmt.Errorf("runtime error: %w\nstdout: %s\nstderr: %s", res.err, stdout.String(), stderr.String())
		}

		// 5. Handle Result & Check for Error Objects
		finalVal := res.val.Interface()
		finalStderr := stderr.String()

		// FIX: If the user code returned an `error` type (e.g. "return err"),
		// we capture it and move it to Stderr so the Tool Handler treats it as a failure.
		if errObj, ok := finalVal.(error); ok {
			if finalStderr != "" {
				finalStderr += "\n"
			}
			finalStderr += "Script returned error: " + errObj.Error()
			finalVal = nil // Nullify value since it was an error
		}

		return CodeModeResult{
			Value:  finalVal,
			Stdout: stdout.String(),
			Stderr: finalStderr,
		}, nil
	}
}

func (s *codeModeStream) Next() (any, error) {
	return s.next()
}

func injectHelpers(i *interp.Interpreter, client utcp.UtcpClientInterface) error {
	// Load standard library — this already includes a perfectly working "context" package
	if err := i.Use(stdlib.Symbols); err != nil {
		return fmt.Errorf("failed to load stdlib: %w", err)
	}

	exports := interp.Exports{
		"codemode_helpers/codemode_helpers": map[string]reflect.Value{
			"CodeModeStream": reflect.ValueOf((*codeModeStream)(nil)),

			"Errorf":  reflect.ValueOf(fmt.Errorf),
			"Sprintf": reflect.ValueOf(fmt.Sprintf),

			"CallTool": reflect.ValueOf(func(name string, args map[string]any) (any, error) {
				return client.CallTool(context.Background(), name, args)
			}),

			"SearchTools": reflect.ValueOf(func(query string, limit int) ([]tools.Tool, error) {
				return client.SearchTools(query, limit)
			}),

			"CallToolStream": reflect.ValueOf(func(name string, args map[string]any) (*codeModeStream, error) {
				stream, err := client.CallToolStream(context.Background(), name, args)
				if err != nil {
					return nil, fmt.Errorf("CallToolStream failed: %w", err)
				}
				return &codeModeStream{next: stream.Next}, nil
			}),
		},
	}

	if err := i.Use(exports); err != nil {
		return fmt.Errorf("failed to export helpers: %w", err)
	}

	return nil
}

func (cm *CodeModeUTCP) createToolHandler() tools.ToolHandler {
	return func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		var args CodeModeArgs
		if code, ok := inputs["code"].(string); ok {
			args.Code = code
		}
		if timeout, ok := inputs["timeout"].(float64); ok { // JSON numbers are float64
			args.Timeout = int(timeout)
		}

		if args.Timeout <= 0 {
			args.Timeout = 3000
		}

		result, err := cm.Execute(ctx, args)
		if err != nil {
			return nil, fmt.Errorf("error executing codemode script: %w", err)
		}

		if result.Stderr != "" {
			return nil, fmt.Errorf("codemode script produced an error: %s", result.Stderr)
		}

		return map[string]interface{}{
			"value":  result.Value,
			"stdout": result.Stdout,
			"stderr": result.Stderr,
		}, nil
	}
}

func normalizeSnippet(code string) string {
	s := strings.TrimSpace(code)

	// If the snippet is a JSON object, convert it to a Go map literal.
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		return "__out = " + jsonToGoLiteral(s)
	}

	// If the snippet assigns a JSON object to __out, convert it.
	if strings.HasPrefix(s, "__out = {") {
		inside := strings.TrimSpace(strings.TrimPrefix(s, "__out = "))
		return "__out = " + jsonToGoLiteral(inside)
	}
	return code
}
