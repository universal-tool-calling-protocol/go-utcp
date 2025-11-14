package codemode

import (
	"context"
	"strings"
	"testing"
	"time"

	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	"github.com/universal-tool-calling-protocol/go-utcp/src/repository"
	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
)

type mockUTCPClient struct {
	callToolFunc       func(context.Context, string, map[string]any) (any, error)
	searchToolsFunc    func(string, int) ([]tools.Tool, error)
	callToolStreamFunc func(context.Context, string, map[string]any) (transports.StreamResult, error)
}

func (m *mockUTCPClient) RegisterToolProvider(ctx context.Context, prov providers.Provider) ([]tools.Tool, error) {
	return nil, nil
}

func (m *mockUTCPClient) DeregisterToolProvider(ctx context.Context, providerName string) error {
	return nil
}

func (m *mockUTCPClient) CallTool(ctx context.Context, toolName string, args map[string]any) (any, error) {
	if m.callToolFunc != nil {
		return m.callToolFunc(ctx, toolName, args)
	}
	return nil, nil
}

func (m *mockUTCPClient) SearchTools(query string, limit int) ([]tools.Tool, error) {
	if m.searchToolsFunc != nil {
		return m.searchToolsFunc(query, limit)
	}
	return nil, nil
}

func (m *mockUTCPClient) GetTransports() map[string]repository.ClientTransport {
	return nil
}

func (m *mockUTCPClient) CallToolStream(ctx context.Context, toolName string, args map[string]any) (transports.StreamResult, error) {
	if m.callToolStreamFunc != nil {
		return m.callToolStreamFunc(ctx, toolName, args)
	}
	return transports.NewSliceStreamResult(nil, nil), nil
}

func TestWrapGoSnippet(t *testing.T) {
	code := "return 42"
	wrapped := wrapGoSnippet(code)

	if !strings.Contains(wrapped, "package main") {
		t.Fatalf("expected wrapper to include package declaration, got %q", wrapped)
	}
	if !strings.Contains(wrapped, code) {
		t.Fatalf("expected original code to be present, got %q", wrapped)
	}
	if !strings.Contains(wrapped, "func __run__() interface{}") {
		t.Fatalf("expected wrapper function, got %q", wrapped)
	}
}

func TestExecuteSuccess(t *testing.T) {
	mode := NewGoCodeMode(&mockUTCPClient{})

	result, err := mode.Execute(`
println("hello from yaegi")
return "value"
`, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Value != "value" {
		t.Fatalf("unexpected value: %v", result.Value)
	}
	if !strings.Contains(result.Stdout, "hello from yaegi") {
		t.Fatalf("expected stdout to contain message, got %q", result.Stdout)
	}
	if result.Stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.Stderr)
	}
}

func TestExecuteEmptyCode(t *testing.T) {
	mode := NewGoCodeMode(&mockUTCPClient{})
	if _, err := mode.Execute("", time.Second); err == nil {
		t.Fatalf("expected error for empty code")
	}
}

func TestExecuteInterpreterError(t *testing.T) {
	mode := NewGoCodeMode(&mockUTCPClient{})
	if _, err := mode.Execute("return unknownVar", time.Second); err == nil {
		t.Fatalf("expected interpreter error")
	}
}

func TestExecuteTimeout(t *testing.T) {
	mode := NewGoCodeMode(&mockUTCPClient{})
	_, err := mode.Execute(`
ch := make(chan struct{})
<-ch
return "never"
`, 50*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !strings.Contains(err.Error(), "execution timeout") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunGoCode(t *testing.T) {
	mode := NewGoCodeMode(&mockUTCPClient{})
	result, err := mode.runGoCode(nil, map[string]any{"code": "return 123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["value"] != 123 {
		t.Fatalf("unexpected value: %v", result["value"])
	}
	if result["stdout"].(string) != "" {
		t.Fatalf("expected empty stdout, got %q", result["stdout"])
	}
	if result["stderr"].(string) != "" {
		t.Fatalf("expected empty stderr, got %q", result["stderr"])
	}
}

func TestRunGoCodePropagatesError(t *testing.T) {
	mode := NewGoCodeMode(&mockUTCPClient{})
	if _, err := mode.runGoCode(nil, map[string]any{"code": ""}); err == nil {
		t.Fatalf("expected error from runGoCode")
	}
}
