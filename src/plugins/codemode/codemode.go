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
	"sync"
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

// Cached minimal stdlib to avoid rebuilding on every execution
var (
	minimalStdlibOnce  sync.Once
	minimalStdlibCache map[string]map[string]reflect.Value
)

func getMinimalStdlib() map[string]map[string]reflect.Value {
	minimalStdlibOnce.Do(func() {
		minimalStdlibCache = map[string]map[string]reflect.Value{}

		// Only load packages that are actually needed by codemode
		neededPackages := []string{
			"context/context",
			"fmt/fmt",
			"reflect/reflect",
		}

		for _, pkg := range neededPackages {
			if symbols, ok := stdlib.Symbols[pkg]; ok {
				minimalStdlibCache[pkg] = symbols
			}
		}
	})
	return minimalStdlibCache
}

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
	code = fixReturnWalrus(code) // Fix 'return x := y' patterns
	code = fixSingleValueCallTool(code)
	code = fixIfAssignment(code) // Fix 'if x, ok = ...' → 'if x, ok := ...'
	code = convertOutWalrus(code)
	code = fixVarWalrus(code)     // Fix 'var x := y' forms
	code = fixRedeclaredErr(code) // Fix 'x, err := ...' followed by 'y, err := ...'
	code = fixBareReturn(code)
	code = ensureOutAssigned(code)
	return code
}

func fixRedeclaredErr(code string) string {
	// This function attempts to fix the common "no new variables on left side of :="
	// error when 'err' is redeclared in subsequent tool calls.
	// It finds the first assignment like `_, err := ...` and then replaces
	// all following instances of `:=` with `=` on lines containing `, err`.
	re := regexp.MustCompile(`(?m)^.*,\s*err\s*:=.*$`)
	firstMatchIndex := re.FindStringIndex(code)

	if firstMatchIndex == nil {
		return code // No 'err' redeclaration pattern found
	}

	restOfCode := code[firstMatchIndex[1]:]
	restOfCode = strings.ReplaceAll(restOfCode, ", err :=", ", err =")
	return code[:firstMatchIndex[1]] + restOfCode
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

func fixIfAssignment(code string) string {
	// Fix 'if x, ok = someFunc()' → 'if x, ok := someFunc()'
	// This is safe because 'if' starts a new scope
	re := regexp.MustCompile(`if\s+(\w+)\s*,\s*(\w+)\s*=\s*`)
	return re.ReplaceAllString(code, "if $1, $2 := ")
}

func fixReturnWalrus(code string) string {
	// Fix invalid short declarations in returns: `return x := y`
	re := regexp.MustCompile(`(?m)^(\s*)return\s+([A-Za-z_]\w*)\s*:=\s*(.+)$`)

	return re.ReplaceAllStringFunc(code, func(line string) string {
		m := re.FindStringSubmatch(line)
		if len(m) != 4 {
			return line
		}
		indent, lhs, rhs := m[1], m[2], strings.TrimSpace(m[3])

		if lhs == "__out" {
			return fmt.Sprintf("%s__out = %s\n%sreturn __out", indent, rhs, indent)
		}

		return fmt.Sprintf("%s%s := %s\n%s__out = %s\n%sreturn __out", indent, lhs, rhs, indent, lhs, indent)
	})
}

func convertOutWalrus(code string) string {
	// Only convert `__out :=` when __out is the sole variable being declared
	// This avoids breaking multi-variable declarations like `result, err := ...`
	// Pattern: __out followed by optional whitespace, :=, but NOT preceded by comma
	// Use capture groups to preserve both leading whitespace and spacing before :=
	re := regexp.MustCompile(`(?m)^(\s*)__out(\s*):=`)
	return re.ReplaceAllString(code, "${1}__out${2}=")
}

func fixVarWalrus(code string) string {
	// Fix 'var x := y' → 'var x = y'
	// Also handles 'var x int := y' and 'var x, y := 1, 2'
	// Limit the match to a single line to avoid consuming following statements
	re := regexp.MustCompile(`(?m)^\s*var\s+([^\n=;]+):=`)
	return re.ReplaceAllString(code, "var $1=")
}

func fixSingleValueCallTool(code string) string {
	// Convert single-value CallTool usage into two-value form that ignores the error.
	// Handles walrus assign, equals assign, and bare return.

	reWalrus := regexp.MustCompile(`(?m)^(\s*)([A-Za-z_]\w*)\s*:=\s*codemode\.CallTool\s*\(`)
	code = reWalrus.ReplaceAllString(code, "${1}${2}, _ := codemode.CallTool(")

	reEquals := regexp.MustCompile(`(?m)^(\s*)([A-Za-z_]\w*)\s*=\s*codemode\.CallTool\s*\(`)
	code = reEquals.ReplaceAllString(code, "${1}${2}, _ = codemode.CallTool(")

	reReturn := regexp.MustCompile(`(?m)^(\s*)return\s+codemode\.CallTool\s*\((.*)\)\s*$`)
	code = reReturn.ReplaceAllString(code, "${1}__tmp, _ := codemode.CallTool($2)\n${1}__out = __tmp\n${1}return __out")

	return code
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
	if strings.Contains(code, "__out") {
		return code
	}

	trim := strings.TrimSpace(code)

	// If we have a single-line var declaration, reuse that identifier
	reVar := regexp.MustCompile(`(?m)^\s*var\s+([A-Za-z_]\w*)\s*=`)
	if m := reVar.FindStringSubmatch(trim); m != nil {
		return code + "\n__out = " + m[1]
	}

	// If it's a single-line assignment (with = or :=), set __out to first LHS identifier
	reAssign := regexp.MustCompile(`(?m)^\s*([A-Za-z_]\w*)(?:\s*,.*)?\s*[:=]=`)
	if m := reAssign.FindStringSubmatch(trim); m != nil {
		return code + "\n__out = " + m[1]
	}

	// If it's a simple single-line expression, assign directly
	if !strings.Contains(trim, "\n") {
		keywords := []string{"var ", "const ", "for ", "if ", "switch ", "select ", "type ", "func ", "go ", "defer ", "return "}
		isKeyword := false
		for _, kw := range keywords {
			if strings.HasPrefix(trim, kw) {
				isKeyword = true
				break
			}
		}
		if !isKeyword {
			return "__out = " + trim
		}
	}

	// Fallback: preserve code and set __out to a neutral value
	return code + "\n__out = nil"
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
	"fmt/fmt"
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
	// OPTIMIZATION: Use cached minimal stdlib instead of loading all stdlib.Symbols
	// This reduces initialization time from ~1.5ms to ~20μs (75x faster)
	if err := i.Use(getMinimalStdlib()); err != nil {
		return fmt.Errorf("failed to load minimal stdlib: %w", err)
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
	return func(ctx context.Context, inputs map[string]interface{}) (any, error) {
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

		return result.Value, nil
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
