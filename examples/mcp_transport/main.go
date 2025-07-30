package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/mcp"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
	mcp "github.com/universal-tool-calling-protocol/go-utcp/src/transports/mcp"
)

func main() {
	// Give providers a moment to start
	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()

	// Create MCPTransport directly
	logger := func(format string, args ...interface{}) {
		fmt.Printf("[MCP] "+format+"\n", args...)
	}
	transport := mcp.NewMCPTransport(logger)

	// Create MCP provider configuration
	mcpProvider := &providers.MCPProvider{
		Name:       "demo_tools",
		Command:    []string{"python3", "-u", "../../scripts/server.py"},
		Args:       []string{},
		Env:        make(map[string]string),
		WorkingDir: ".",
		StdinData:  "",
		Timeout:    30,
	}

	// Register the tool provider and discover tools
	tools, err := transport.RegisterToolProvider(ctx, mcpProvider)
	if err != nil {
		log.Fatalf("failed to register MCP provider: %v", err)
	}

	if len(tools) == 0 {
		log.Fatal("no tools found")
	}

	fmt.Println("Tools were found:")
	for _, t := range tools {
		fmt.Printf(" - %s: %s\n", t.Name, t.Description)
	}
	if len(tools) != 2 {
		log.Fatalf("expected exactly two tools, got %d", len(tools))
	}

	argsMap := map[string]any{"name": "Kamil"}

	res, err := transport.CallTool(ctx, tools[0].Name, argsMap, mcpProvider, false)
	fmt.Println(res.(map[string]any))
	res, err = transport.CallTool(ctx, tools[1].Name, argsMap, mcpProvider, base.CallingOptions{
		Stream: true,
	})

	if err != nil {
		log.Fatalf("stream call error: %v", err)
	}
	sub, ok := res.(*transports.ChannelStreamResult)
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
