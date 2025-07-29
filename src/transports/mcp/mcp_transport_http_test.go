package mcp

import (
	"context"
	"fmt"
	"io"
	"log"
	"testing"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cast"

	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/mcp"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
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
	log.Println(m)
	content, ok := m["text"]
	if !ok && content != "Hello, Go!" {
		t.Fatalf("unexpected result: %#v", m)
	}

	if err := tr.DeregisterToolProvider(ctx, prov); err != nil {
		t.Fatalf("deregister err: %v", err)
	}
}

// startTestHTTPStreamServer starts a simple MCP HTTP server with a streaming tool.
func startTestHTTPStreamServer(addr string) *mcpserver.StreamableHTTPServer {
	srv := mcpserver.NewMCPServer("demo", "1.0.0")
	count := mcp.NewTool("count", mcp.WithNumber("n"))
	srv.AddTool(count, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		n := cast.ToInt(req.GetArguments()["n"])
		if n <= 0 {
			n = 2
		}
		httpSrv := mcpserver.ServerFromContext(ctx)
		for i := 1; i <= n; i++ {
			_ = httpSrv.SendNotificationToClient(ctx, "count", map[string]any{"value": i})
			time.Sleep(50 * time.Millisecond)
		}
		return mcp.NewToolResultText("done"), nil
	})
	httpSrv := mcpserver.NewStreamableHTTPServer(srv)
	go func() { _ = httpSrv.Start(addr) }()
	time.Sleep(100 * time.Millisecond)
	return httpSrv
}

func TestMCPHTTPStreamReturnsStreamResult(t *testing.T) {
	httpSrv := startTestHTTPStreamServer(":8099")
	defer httpSrv.Shutdown(context.Background())

	tr := NewMCPTransport(nil)
	prov := &providers.MCPProvider{
		Name: "demo",
		URL:  "http://localhost:8099/mcp",
	}

	ctx := context.Background()
	if _, err := tr.RegisterToolProvider(ctx, prov); err != nil {
		t.Fatalf("register err: %v", err)
	}
	ctxWithCT := context.WithValue(ctx, "contentType", "event-stream")

	res, err := tr.CallToolStream(ctxWithCT, "count", map[string]any{"n": 3}, prov)
	if err != nil {
		t.Fatalf("call err: %v", err)
	}

	sr, ok := res.(transports.StreamResult)
	if !ok {
		t.Fatalf("expected StreamResult, got %T", res)
	}

	var count int
	for {
		item, err := sr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("next err: %v", err)
		}
		if m, ok := item.(map[string]any); ok && m["method"] == "count" {
			if p, ok := m["params"].(mcp.NotificationParams); ok {
				if _, ok := p.AdditionalFields["value"]; ok {
					count++
				}
			} else if mp, ok := m["params"].(map[string]any); ok {
				if _, ok := mp["value"]; ok {
					count++
				}
			}
		}
	}
	if count != 3 {
		t.Fatalf("expected 3 notifications, got %d", count)
	}
	sr.Close()

	if err := tr.DeregisterToolProvider(ctx, prov); err != nil {
		t.Fatalf("deregister err: %v", err)
	}
}
