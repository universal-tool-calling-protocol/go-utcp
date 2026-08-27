package codemode

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	"github.com/universal-tool-calling-protocol/go-utcp/src/repository"
	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
)

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
func (m *mockUTCP) DeregisterToolProvider(ctx context.Context, providerName string) error { return nil }
func (m *mockUTCP) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
	if m.callToolFn == nil {
		return nil, errors.New("CallTool not configured")
	}
	return m.callToolFn(name, args)
}
func (m *mockUTCP) SearchTools(query string, limit int) ([]tools.Tool, error) {
	if m.searchToolsFn == nil {
		return nil, nil
	}
	return m.searchToolsFn(query, limit)
}
func (m *mockUTCP) GetTransports() map[string]repository.ClientTransport { return nil }
func (m *mockUTCP) CallToolStream(ctx context.Context, name string, args map[string]any) (transports.StreamResult, error) {
	if m.callToolStreamFn == nil {
		return nil, errors.New("CallToolStream not configured")
	}
	return m.callToolStreamFn(name, args)
}

func TestCodeModeExecuteExprArithmetic(t *testing.T) {
	cm := NewCodeModeUTCP(&mockUTCP{}, nil)
	result, err := cm.Execute(context.Background(), CodeModeArgs{Code: `2 + 3`, Timeout: 2000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Value != 5 {
		t.Fatalf("expected 5, got %#v", result.Value)
	}
}

func TestCodeModeExecuteExprSequence(t *testing.T) {
	cm := NewCodeModeUTCP(&mockUTCP{}, nil)
	result, err := cm.Execute(context.Background(), CodeModeArgs{
		Code: `let a = 2; let b = 3; a * b`,
		Timeout: 2000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Value != 6 {
		t.Fatalf("expected 6, got %#v", result.Value)
	}
}

func TestCodeModeExecuteCallTool(t *testing.T) {
	mock := &mockUTCP{callToolFn: func(name string, args map[string]any) (any, error) {
		if name != "math.add" {
			return nil, errors.New("unexpected tool")
		}
		return map[string]any{"result": 9}, nil
	}}

	cm := NewCodeModeUTCP(mock, nil)
	result, err := cm.Execute(context.Background(), CodeModeArgs{
		Code: `codemode.CallTool("math.add", {"a": 4, "b": 5})`,
		Timeout: 2000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := result.Value.(map[string]any)
	if !ok || got["result"] != 9 {
		t.Fatalf("unexpected result: %#v", result.Value)
	}
}

func TestCodeModeExecuteToolChain(t *testing.T) {
	mock := &mockUTCP{callToolFn: func(name string, args map[string]any) (any, error) {
		switch name {
		case "math.add":
			return map[string]any{"result": args["a"].(int) + args["b"].(int)}, nil
		case "math.multiply":
			return map[string]any{"result": args["value"].(int) * args["factor"].(int)}, nil
		default:
			return nil, errors.New("unknown tool")
		}
	}}

	cm := NewCodeModeUTCP(mock, nil)
	result, err := cm.Execute(context.Background(), CodeModeArgs{
		Code: `let r1 = codemode.CallTool("math.add", {"a": 4, "b": 5}); let value = codemode.Get(r1, "result"); let r2 = codemode.CallTool("math.multiply", {"value": value, "factor": 2}); r2`,
		Timeout: 2000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := result.Value.(map[string]any)
	if !ok || got["result"] != 18 {
		t.Fatalf("unexpected chained result: %#v", result.Value)
	}
}

func TestCodeModeExecuteStream(t *testing.T) {
	mock := &mockUTCP{callToolStreamFn: func(name string, args map[string]any) (transports.StreamResult, error) {
		return &mockStream{items: []any{"hello", "world"}}, nil
	}}

	cm := NewCodeModeUTCP(mock, nil)
	result, err := cm.Execute(context.Background(), CodeModeArgs{
		Code: `codemode.CallToolStream("stream.echo", {"value": "ignored"})`,
		Timeout: 2000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items, ok := result.Value.([]any)
	if !ok || len(items) != 2 || items[0] != "hello" || items[1] != "world" {
		t.Fatalf("unexpected stream result: %#v", result.Value)
	}
}

func TestCodeModeExecuteToolError(t *testing.T) {
	mock := &mockUTCP{callToolFn: func(name string, args map[string]any) (any, error) {
		return nil, errors.New("tool failed")
	}}
	cm := NewCodeModeUTCP(mock, nil)
	_, err := cm.Execute(context.Background(), CodeModeArgs{
		Code: `codemode.CallTool("broken.tool", {})`,
		Timeout: 2000,
	})
	if err == nil {
		t.Fatal("expected tool error")
	}
}

func TestCodeModeExecuteTimeout(t *testing.T) {
	mock := &mockUTCP{callToolFn: func(name string, args map[string]any) (any, error) {
		<-time.After(500 * time.Millisecond)
		return "late", nil
	}}
	cm := NewCodeModeUTCP(mock, nil)
	start := time.Now()
	_, err := cm.Execute(context.Background(), CodeModeArgs{
		Code: `codemode.CallTool("slow.tool", {})`,
		Timeout: 50,
	})
	if err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(start) > 300*time.Millisecond {
		t.Fatalf("timeout took too long: %s", time.Since(start))
	}
}

func TestExtractGeneratedToolNamesExpr(t *testing.T) {
	code := `let a = codemode.CallTool("math.add", {"a": 1}); codemode.CallToolStream("stream.echo", {})`
	got := extractGeneratedToolNames(code)
	if len(got) != 2 || got[0] != "math.add" || got[1] != "stream.echo" {
		t.Fatalf("unexpected tools: %#v", got)
	}
}
