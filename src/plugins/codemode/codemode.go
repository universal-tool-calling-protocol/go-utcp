// path: codemode/codemode_utcp.go
package codemode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
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
}

func NewCodeModeUTCP(client utcp.UtcpClientInterface) *CodeModeUTCP {
	return &CodeModeUTCP{client: client}
}

//
// ───────────────────────────────────────────────────────────
//   Register UTCP Tool
// ───────────────────────────────────────────────────────────
//

func (c *CodeModeUTCP) Tools(ctx context.Context) ([]tools.Tool, error) {
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

			Handler: createToolHandler(c.client),
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
	code = preprocessUserCode(code)
	clean := normalizeSnippet(code)

	return wrapIntoProgram(clean), nil
}

func preprocessUserCode(code string) string {
	trim := strings.TrimSpace(code)

	if strings.HasPrefix(trim, "{") {
		code = "__out = " + jsonToGoMap(trim)
	}

	code = stripOutRedeclarations(code)
	code = convertOutWalrus(code)
	code = ensureOutAssigned(code)

	return code
}

func stripOutRedeclarations(code string) string {
	re := regexp.MustCompile(`(?m)^\s*(var\s+__out\s+.*|__out\s*:=.*)$`)
	return re.ReplaceAllString(code, "")
}

func convertOutWalrus(code string) string {
	re := regexp.MustCompile(`__out\s*:=`)
	return re.ReplaceAllString(code, "__out = ")
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
    "fmt"
    codemode "codemode_helpers/codemode_helpers"
)

func run() any {
    var __out any

    %s

    return __out
}
`, clean)
}
func withTimeout(ctx context.Context, ms int) (context.Context, context.CancelFunc) {
	if ms <= 0 {
		ms = 5000
	}
	return context.WithTimeout(ctx, time.Duration(ms)*time.Millisecond)
}

func (c *CodeModeUTCP) Execute(ctx context.Context, args CodeModeArgs) (CodeModeResult, error) {
	ctx, cancel := withTimeout(ctx, args.Timeout)
	defer cancel()

	i, stdout, stderr := newInterpreter()

	if err := c.injectHelpers(ctx, i); err != nil {
		return CodeModeResult{}, fmt.Errorf("failed to inject helpers: %w", err)
	}

	wrapped, err := c.prepareWrappedProgram(args.Code)
	if err != nil {
		return CodeModeResult{}, err
	}

	if _, err := i.EvalWithContext(ctx, wrapped); err != nil {
		return CodeModeResult{}, fmt.Errorf("code execution failed: %w\n%s", err, stderr.String())
	}

	v, err := i.EvalWithContext(ctx, "main.run()")
	if err != nil {
		return CodeModeResult{}, fmt.Errorf("failed to get return value: %w\n%s", err, stderr.String())
	}

	return CodeModeResult{
		Value:  v.Interface(),
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}, nil
}

func (s *codeModeStream) Next() (any, error) {
	return s.next()
}

func (c *CodeModeUTCP) injectHelpers(ctx context.Context, i *interp.Interpreter) error {
	// Load standard library ONLY ONCE
	if err := i.Use(stdlib.Symbols); err != nil {
		return fmt.Errorf("codemode: failed to load stdlib: %w", err)
	}

	// Register helper namespace
	if err := i.Use(interp.Exports{
		"codemode_helpers/codemode_helpers": {
			"CodeModeStream": reflect.ValueOf(codeModeStream{}),
			"Errorf": reflect.ValueOf(func(format string, args ...any) error {
				return fmt.Errorf(format, args...)
			}),
			"CallTool": reflect.ValueOf(func(name string, args map[string]any) (any, error) {
				v, err := c.client.CallTool(ctx, name, args)
				if err != nil {
					return nil, fmt.Errorf("codemode CallTool(%s) failed: %w", name, err)
				}
				return v, nil
			}),

			"SearchTools": reflect.ValueOf(func(query string, limit int) ([]tools.Tool, error) {
				v, err := c.client.SearchTools(query, limit)
				if err != nil {
					return nil, fmt.Errorf("codemode SearchTools(%s) failed: %w", query, err)
				}
				return v, nil
			}),

			"CallToolStream": reflect.ValueOf(func(name string, args map[string]any) (*codeModeStream, error) {
				s, err := c.client.CallToolStream(ctx, name, args)
				if err != nil {
					return nil, fmt.Errorf("codemode CallToolStream(%s) failed: %w", name, err)
				}

				return &codeModeStream{
					next: func() (any, error) {
						v, err := s.Next()
						if err != nil {
							return nil, fmt.Errorf("codemode stream.Next(%s) failed: %w", name, err)
						}
						return v, nil
					},
				}, nil
			}),
		},
	}); err != nil {
		return fmt.Errorf("codemode: failed to load exports: %w", err)
	}

	return nil
}

func createToolHandler(client utcp.UtcpClientInterface) tools.ToolHandler {
	return func(ctx map[string]interface{}, inputs map[string]interface{}) (map[string]interface{}, error) {
		if client == nil {
			return nil, fmt.Errorf("codemode tool handler was created without a valid UTCP client")
		}

		// We create a temporary instance here to call Execute, but it uses the captured client.
		c := NewCodeModeUTCP(client)
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

		result, err := c.Execute(context.Background(), args)
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

	// If the snippet *is* a JSON object → convert to Go map
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		var m map[string]any
		if json.Unmarshal([]byte(s), &m) == nil {
			// %#v prints Go syntax: map[string]interface {}{"a":1}
			return "__out = " + fmt.Sprintf("%#v", m)
		}
	}

	// If snippet *assigns* JSON to __out: `__out = {...}`
	if strings.HasPrefix(s, "__out = {") {
		var m map[string]any
		inside := strings.TrimSpace(strings.TrimPrefix(s, "__out = "))
		if json.Unmarshal([]byte(inside), &m) == nil {
			return "__out = " + fmt.Sprintf("%#v", m)
		}
	}

	return code
}

// jsonToGoMap converts a JSON object string into a Go map literal.
func jsonToGoMap(s string) string {
	s = strings.TrimSpace(s)

	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		// return original snippet if not valid JSON
		return s
	}

	// %#v prints Go map syntax like: map[string]interface {}{"a":1}
	return fmt.Sprintf("%#v", m)
}
