package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cast"

	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/mcp"
	transports "github.com/universal-tool-calling-protocol/go-utcp/src/transports"
	mcp "github.com/universal-tool-calling-protocol/go-utcp/src/transports/mcp"
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
	logger := func(format string, args ...interface{}) { log.Printf(format, args...) }
	transport := mcp.NewMCPTransport(logger)

	// 3) Configure provider using URL only
	provider := &providers.MCPProvider{
		Name: "demo-http",
		URL:  "http://localhost:8082/mcp",
	}

	ctx := context.Background()

	// 4) Register provider and list tools
	tools, err := transport.RegisterToolProvider(ctx, provider)
	if err != nil {
		log.Fatalf("register error: %v", err)
	}
	log.Printf("Discovered %d tools", len(tools))
	for _, t := range tools {
		log.Printf(" - %s: %s", t.Name, t.Description)
	}

	// 5) Call hello tool
	result, err := transport.CallTool(ctx, "hello", map[string]any{"name": "Go"}, provider, nil)
	if err != nil {
		log.Fatalf("call error: %v", err)
	}
	fmt.Printf("Hello result: %#v\n", result)

	// 6) Stream results from count_stream
	result, err = transport.CallToolStream(ctx, "count_stream", map[string]any{"count": 5}, provider)
	if err != nil {
		log.Fatalf("stream call error: %v", err)
	}
	sub, ok := result.(*transports.ChannelStreamResult)
	if !ok {
		log.Fatalf("unexpected subscription type: %T", res)
	}
	for {
		val, err := sub.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("subscription next error: %v", err)
		}
		fmt.Printf("Subscription update: %#v\n", val)
	}
	sub.Close()
}
