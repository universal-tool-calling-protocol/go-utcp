// path: codemode/codemode_utcp.go
package codemode

import (
	"bytes"
	"context"
	"fmt"
	"reflect"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"

	"time"

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

			Handler: c.toolHandler,
		},
	}, nil
}

//
// ───────────────────────────────────────────────────────────
//   EXECUTE DSL USING AST
// ───────────────────────────────────────────────────────────
//

func (c *CodeModeUTCP) Execute(ctx context.Context, args CodeModeArgs) (CodeModeResult, error) {
	timeout := time.Duration(args.Timeout) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Initialize Yaegi interpreter
	var stdout, stderr bytes.Buffer
	i := interp.New(interp.Options{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	i.Use(stdlib.Symbols)

	// Make UTCP functions available to the script
	if err := c.injectHelpers(ctx, i); err != nil {
		return CodeModeResult{}, fmt.Errorf("failed to inject helpers: %w", err)
	}

	// Wrap user code in a function to capture the return value
	// In Execute(), remove the import line:
	wrappedCode := fmt.Sprintf(`package main

import "codemode"  // <--- Add this to resolve codemode

func run() any {
    %s
    return nil
}`, args.Code)
	if _, err := i.EvalWithContext(ctx, wrappedCode); err != nil {
		return CodeModeResult{}, fmt.Errorf("code execution failed: %w\n%s", err, stderr.String())
	}

	// Get the result from the `run` function
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

// injectHelpers makes UTCP client functions available to the Yaegi interpreter.
type codeModeStream struct {
	next func() (any, error)
}

func (s *codeModeStream) Next() (any, error) {
	return s.next()
}

func (c *CodeModeUTCP) injectHelpers(ctx context.Context, i *interp.Interpreter) error {
	i.Use(interp.Exports{
		"codemode/codemode": { // must match: import "codemode"
			"codeModeStream": reflect.ValueOf((*codeModeStream)(nil)).Elem(),

			"CallTool": reflect.ValueOf(func(name string, args map[string]any) (any, error) {
				return c.client.CallTool(ctx, name, args)
			}),

			"SearchTools": reflect.ValueOf(func(query string, limit int) ([]tools.Tool, error) {
				return c.client.SearchTools(query, limit)
			}),

			"CallToolStream": reflect.ValueOf(func(name string, args map[string]any) (*codeModeStream, error) {
				s, err := c.client.CallToolStream(ctx, name, args)
				if err != nil {
					return nil, err
				}

				return &codeModeStream{
					next: func() (any, error) {
						return s.Next()
					},
				}, nil
			}),
		},
	})

	return nil
}

func (c *CodeModeUTCP) toolHandler(ctx map[string]interface{}, inputs map[string]interface{}) (map[string]interface{}, error) {
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
		return nil, err
	}

	// To prevent issues with JSON marshaling of complex types, we can format the value.
	// For now, we'll just pass it through.
	return map[string]interface{}{
		"value":  result.Value,
		"stdout": result.Stdout,
		"stderr": result.Stderr,
	}, nil
}
