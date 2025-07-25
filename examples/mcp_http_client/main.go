package main

import (
	"context"
	"fmt"
	"log"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cast"
	"github.com/universal-tool-calling-protocol/go-utcp"
)

// startMCPHTTPServer launches a simple MCP HTTP server with a hello tool and a streaming count tool.
func startMCPHTTPServer(addr string) {
	srv := mcpserver.NewMCPServer("demo-mcp", "1.0.0")

	// hello tool
	helloTool := mcp.NewTool("hello",
		mcp.WithDescription("Return a greeting"),
		mcp.WithString("name", mcp.Description("Name to greet")),
	)
	srv.AddTool(helloTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.GetArguments()["name"].(string)
		if name == "" {
			name = "World"
		}
		return mcp.NewToolResultText(fmt.Sprintf("Hello, %s!", name)), nil
	})

	// streaming count tool
	countTool := mcp.NewTool("count_stream",
		mcp.WithNumber("count", mcp.Description("How many numbers to stream")),
	)
	srv.AddTool(countTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		n := cast.ToInt(req.GetArguments()["count"])
		if n <= 0 {
			n = 3
		}
		httpSrv := mcpserver.ServerFromContext(ctx)
		for i := 1; i <= n; i++ {
			_ = httpSrv.SendNotificationToClient(ctx, "count", map[string]any{"value": i})
			time.Sleep(200 * time.Millisecond)
		}
		return mcp.NewToolResultText("done"), nil
	})

	httpServer := mcpserver.NewStreamableHTTPServer(srv)
	go func() {
		if err := httpServer.Start(addr); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()
}

func main() {
	// 1) Start the MCP HTTP server
	startMCPHTTPServer(":8082")
	time.Sleep(200 * time.Millisecond)

	// 2) Build transport
	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("client error: %v", err)
	}

	// Discover tools
	tools, err := client.SearchTools("", 10)
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	fmt.Printf("Discovered %d tools:\n", len(tools))
	for _, t := range tools {
		fmt.Printf(" - %s\n", t.Name)
	}

	// 5) Call hello tool
	result, err := client.CallTool(ctx, "demo_tools.hello", map[string]any{"name": "Go"})
	if err != nil {
		log.Fatalf("call error: %v", err)
	}
	fmt.Printf("Hello result: %#v\n", result)
	result, err = client.CallTool(ctx, "demo_tools.count_stream", map[string]any{})
	if err != nil {
		log.Fatalf("call error: %v", err)
	}
	fmt.Printf("Hello result: %#v\n", result)
}
