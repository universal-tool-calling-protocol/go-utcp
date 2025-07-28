package mcp

import (
	"context"
	"fmt"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cast"

	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/mcp"
)

// startTestHTTPServer starts a simple MCP HTTP server on the given addr.
func startTestHTTPServer(addr string) *mcpserver.StreamableHTTPServer {
	srv := mcpserver.NewMCPServer("demo", "1.0.0")
	hello := mcp.NewTool("hello", mcp.WithString("name"))
	srv.AddTool(hello, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := cast.ToString(req.GetArguments()["name"])
		if name == "" {
			name = "World"
		}
		return mcp.NewToolResultText(fmt.Sprintf("Hello, %s!", name)), nil
	})
	httpSrv := mcpserver.NewStreamableHTTPServer(srv)
	go func() { _ = httpSrv.Start(addr) }()
	// wait briefly for server to start
	time.Sleep(100 * time.Millisecond)
	return httpSrv
}

func TestMCPHTTPNonStreamReturnsMap(t *testing.T) {
	httpSrv := startTestHTTPServer(":8098")
	defer httpSrv.Shutdown(context.Background())

	tr := NewMCPTransport(nil)
	prov := &providers.MCPProvider{Name: "demo", URL: "http://localhost:8098/mcp"}

	ctx := context.Background()
	if _, err := tr.RegisterToolProvider(ctx, prov); err != nil {
		t.Fatalf("register err: %v", err)
	}

	res, err := tr.CallTool(ctx, "hello", map[string]any{"name": "Go"}, prov, nil)
	if err != nil {
		t.Fatalf("call err: %v", err)
	}

	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", res)
	}

	content, ok := m["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("unexpected result: %#v", m)
	}
	first, ok := content[0].(map[string]any)
	if !ok || first["text"] != "Hello, Go!" {
		t.Fatalf("unexpected result: %#v", m)
	}

	if err := tr.DeregisterToolProvider(ctx, prov); err != nil {
		t.Fatalf("deregister err: %v", err)
	}
}
