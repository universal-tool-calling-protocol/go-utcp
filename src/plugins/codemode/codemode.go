package codemode

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

// Tool name
const GoCodeModeToolName = "codemode.run_go"

// Input schema
type GoCodeModeArgs struct {
	Code string `json:"code" jsonschema:"Inline Go code to execute"`
}

// Output schema
type GoCodeModeResult struct {
	Value  any    `json:"value"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

// Runtime
type GoCodeMode struct {
	client utcp.UtcpClientInterface
}

// Constructor
func NewGoCodeMode(client utcp.UtcpClientInterface) *GoCodeMode {
	return &GoCodeMode{client: client}
}

//
// ─────────────────────────────────────────────────────────────
//   Register codemode.run_go as a UTCP Tool
// ─────────────────────────────────────────────────────────────
//

func (c *GoCodeMode) RegisterTool() tools.Tool {
	return tools.Tool{
		Name:        GoCodeModeToolName,
		Description: "Execute Go code with access to UTCP tools",
		Inputs: tools.ToolInputOutputSchema{
			Type: "object",
			Properties: map[string]any{
				"code": map[string]any{
					"type":        "string",
					"description": "Go snippet to execute using Yaegi",
				},
			},
			Required: []string{"code"},
		},
		Outputs: tools.ToolInputOutputSchema{
			Type: "object",
			Properties: map[string]any{
				"value":  map[string]any{"type": "string"},
				"stdout": map[string]any{"type": "string"},
				"stderr": map[string]any{"type": "string"},
			},
		},
		Handler: c.runGoCode,
	}
}

//
// ─────────────────────────────────────────────────────────────
//   UTCP Tool Handler
// ─────────────────────────────────────────────────────────────
//

func (c *GoCodeMode) runGoCode(ctx map[string]any, inputs map[string]any) (map[string]any, error) {
	code, _ := inputs["code"].(string)
	result, err := c.Execute(code, 10*time.Second)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"value":  result.Value,
		"stdout": result.Stdout,
		"stderr": result.Stderr,
	}, nil
}

//
// ─────────────────────────────────────────────────────────────
//   Core Execution: Execute Go Code via Yaegi
// ─────────────────────────────────────────────────────────────
//

func (c *GoCodeMode) Execute(code string, timeout time.Duration) (*GoCodeModeResult, error) {
	if code == "" {
		return nil, fmt.Errorf("empty code")
	}

	var stdoutBuf, stderrBuf bytes.Buffer

	// Create Yaegi interpreter
	i := interp.New(interp.Options{
		Stdout: &stdoutBuf,
		Stderr: &stderrBuf,
	})
	i.Use(stdlib.Symbols)

	// Inject UTCP helper functions
	c.injectHelpers(i)

	// Wrap snippet
	wrapped := wrapGoSnippet(code)

	// Timeout context
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ch := make(chan struct {
		val any
		err error
	}, 1)

	go func() {
		// Compile wrapper
		if _, err := i.Eval(wrapped); err != nil {
			ch <- struct {
				val any
				err error
			}{nil, err}
			return
		}

		// Execute __run__()
		v, err := i.Eval("__run__()")
		if err != nil {
			ch <- struct {
				val any
				err error
			}{nil, err}
			return
		}

		ch <- struct {
			val any
			err error
		}{v.Interface(), nil}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("execution timeout: %s", timeout)
	case out := <-ch:
		return &GoCodeModeResult{
			Value:  out.val,
			Stdout: stdoutBuf.String(),
			Stderr: stderrBuf.String(),
		}, out.err
	}
}

//
// ─────────────────────────────────────────────────────────────
//   Inject callTool / searchTools / callToolStream into Yaegi
// ─────────────────────────────────────────────────────────────
//

func (c *GoCodeMode) injectHelpers(i *interp.Interpreter) {
	i.Use(interp.Exports{
		"main": map[string]reflect.Value{
			// callTool("provider.tool", map[string]any)
			"callTool": reflect.ValueOf(
				func(name string, args map[string]any) (any, error) {
					return c.client.CallTool(context.Background(), name, args)
				},
			),
			"CallTool": reflect.ValueOf(
				func(name string, args map[string]any) (any, error) {
					return c.client.CallTool(context.Background(), name, args)
				},
			),

			// searchTools("query", 5)
			"searchTools": reflect.ValueOf(
				func(query string, limit int) ([]tools.Tool, error) {
					return c.client.SearchTools(query, limit)
				},
			),
			"SearchTools": reflect.ValueOf(
				func(query string, limit int) ([]tools.Tool, error) {
					return c.client.SearchTools(query, limit)
				},
			),

			// callToolStream("provider.tool", args)
			"callToolStream": reflect.ValueOf(
				func(name string, args map[string]any) (string, error) {
					stream, err := c.client.CallToolStream(context.Background(), name, args)
					if err != nil {
						return "", err
					}
					var buf bytes.Buffer
					for {
						chunk, err := stream.Next()
						if err != nil {
							break
						}
						if s, ok := chunk.(string); ok {
							buf.WriteString(s)
						}
					}
					return buf.String(), nil
				},
			),
			"CallToolStream": reflect.ValueOf(
				func(name string, args map[string]any) (string, error) {
					stream, err := c.client.CallToolStream(context.Background(), name, args)
					if err != nil {
						return "", err
					}
					var buf bytes.Buffer
					for {
						chunk, err := stream.Next()
						if err != nil {
							break
						}
						if s, ok := chunk.(string); ok {
							buf.WriteString(s)
						}
					}
					return buf.String(), nil
				},
			),
		},
	})
}

//
// ─────────────────────────────────────────────────────────────
//   Wrap Code Snippet in __run__ Function
// ─────────────────────────────────────────────────────────────
//

func wrapGoSnippet(code string) string {
	return fmt.Sprintf(`
package main

func __run__() interface{} {
	%s
}
`, code)
}
