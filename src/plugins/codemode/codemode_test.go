// path: codemode/codemode_utcp_test.go
package codemode

import (
	"context"
	"errors"
	"testing"

	"github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	"github.com/universal-tool-calling-protocol/go-utcp/src/repository"
	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"

	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
)

//
// ─────────────────────────────────────────────────────────────
//   Mock UTCP Client
// ─────────────────────────────────────────────────────────────
//

type mockStream struct {
	items []any
	index int
}

func (m *mockStream) Next() (any, error) {
	if m.index >= len(m.items) {
		return nil, errors.New("EOF")
	}
	item := m.items[m.index]
	m.index++
	return item, nil
}

func (m *mockStream) Close() error { return nil }

type mockUTCP struct {
	callToolFn       func(name string, args map[string]any) (any, error)
	callToolStreamFn func(name string, args map[string]any) (transports.StreamResult, error)
	searchToolsFn    func(query string, limit int) ([]tools.Tool, error)
}

func (m *mockUTCP) RegisterToolProvider(ctx context.Context, prov base.Provider) ([]tools.Tool, error) {
	return nil, nil
}
func (m *mockUTCP) DeregisterToolProvider(ctx context.Context, providerName string) error {
	return nil
}
func (m *mockUTCP) CallTool(ctx context.Context, toolName string, args map[string]any) (any, error) {
	return m.callToolFn(toolName, args)
}
func (m *mockUTCP) SearchTools(query string, limit int) ([]tools.Tool, error) {
	return m.searchToolsFn(query, limit)
}
func (m *mockUTCP) GetTransports() map[string]repository.ClientTransport {
	return nil
}

func (m *mockUTCP) CallToolStream(ctx context.Context, toolName string, args map[string]any) (transports.StreamResult, error) {
	return m.callToolStreamFn(toolName, args)
}

// toFloat64 handles conversion from int or float64.
func toFloat64(v any) (float64, bool) {
	if f, ok := v.(float64); ok {
		return f, true
	}
	if i, ok := v.(int); ok {
		return float64(i), true
	}
	// JSON unmarshals numbers into float64 by default
	return 0, false
}

//
// ─────────────────────────────────────────────────────────────
//   TESTS
// ─────────────────────────────────────────────────────────────
//

func TestCodeMode_Execute_Simple(t *testing.T) {
	mock := &mockUTCP{}
	cm := NewCodeModeUTCP(mock)

	res, err := cm.Execute(context.Background(), CodeModeArgs{
		Code:    `__out = 2 + 3`,
		Timeout: 2000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Value.(int) != 5 {
		t.Fatalf("expected 5, got %#v", res.Value)
	}
}

func TestCodeMode_Execute_Timeout(t *testing.T) {
	mock := &mockUTCP{}
	cm := NewCodeModeUTCP(mock)

	_, err := cm.Execute(context.Background(), CodeModeArgs{
		Code: `
            for {
            }
        `,
		Timeout: 50,
	})
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
}

func TestCodeMode_Execute_CallTool(t *testing.T) {
	mock := &mockUTCP{
		callToolFn: func(name string, args map[string]any) (any, error) {
			if name != "math.add" {
				t.Fatalf("unexpected tool name: %s", name)
			}
			return map[string]any{"result": 9}, nil
		},
	}

	cm := NewCodeModeUTCP(mock)

	res, err := cm.Execute(context.Background(), CodeModeArgs{
		Code: `
            out, _ := codemode.CallTool("math.add", map[string]any{
                "a": 4,
                "b": 5,
            })
            __out = out
        `,
		Timeout: 2000,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := res.Value.(map[string]any)
	if !ok {
		t.Fatalf("expected a map, got %T", res.Value)
	}

	val, ok := toFloat64(resultMap["result"])
	if !ok {
		t.Fatalf("result is not a number: %#v", resultMap["result"])
	}

	if val != 9 {
		t.Fatalf("expected result 9, got %v", val)
	}
}

func TestCodeMode_Execute_MultipleCallTool(t *testing.T) {
	mock := &mockUTCP{
		callToolFn: func(name string, args map[string]any) (any, error) {
			a, _ := toFloat64(args["a"])
			b, _ := toFloat64(args["b"])

			switch name {
			case "math.add":
				return map[string]any{"result": a + b}, nil
			case "math.multiply":
				return map[string]any{"result": a * b}, nil
			default:
				return nil, errors.New("unknown tool")
			}
		},
	}

	cm := NewCodeModeUTCP(mock)

	res, err := cm.Execute(context.Background(), CodeModeArgs{
		Code: `
			addRes, _ := codemode.CallTool("math.add", map[string]any{"a": 4, "b": 5})
			intermediate := addRes.(map[string]any)["result"].(float64)
			multRes, _ := codemode.CallTool("math.multiply", map[string]any{"a": intermediate, "b": 2})
			__out = multRes
`,
		Timeout: 2000,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := res.Value.(map[string]any)
	if !ok {
		t.Fatalf("expected a map, got %T", res.Value)
	}

	if resultMap["result"] != float64(18) {
		t.Fatalf("expected result 18, got %#v", resultMap["result"])
	}
}

func TestCodeMode_Execute_SearchTools(t *testing.T) {
	mock := &mockUTCP{
		searchToolsFn: func(query string, limit int) ([]tools.Tool, error) {
			return []tools.Tool{
				{Name: "memory.store"},
				{Name: "memory.get"},
			}, nil
		},
	}

	cm := NewCodeModeUTCP(mock)

	res, err := cm.Execute(context.Background(), CodeModeArgs{
		Code: `
            ts, _ := codemode.SearchTools("memory", 10)
            __out = ts
        `,
		Timeout: 2000,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultSlice, ok := res.Value.([]tools.Tool)
	if !ok {
		t.Fatalf("expected a []tools.Tool slice, got %T", res.Value)
	}

	if len(resultSlice) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(resultSlice))
	}
}

func TestCodeMode_Execute_CallToolStream(t *testing.T) {
	mock := &mockUTCP{
		callToolStreamFn: func(name string, args map[string]any) (transports.StreamResult, error) {
			return &mockStream{
				items: []any{
					"hello",
					"world",
				},
			}, nil
		},
	}

	cm := NewCodeModeUTCP(mock)

	res, err := cm.Execute(context.Background(), CodeModeArgs{
		Code: `
    stream, _ := codemode.CallToolStream("stream.echo", map[string]any{
        "value": "ignored",
    })
    var result string
    chunk, _ := stream.Next()
    for ; chunk != nil; {
        result += chunk.(string)
        chunk, _ = stream.Next()
    }
    __out = result
`,
		Timeout: 2000,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Value != "helloworld" {
		t.Fatalf("expected 'helloworld', got %#v", res.Value)
	}
}
