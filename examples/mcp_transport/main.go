package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/mcp"
	transports "github.com/universal-tool-calling-protocol/go-utcp/src/transports/mcp"
)

func main() {
	// Give providers a moment to start
	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()

	// Create MCPTransport directly
	logger := func(format string, args ...interface{}) {
		fmt.Printf("[MCP] "+format+"\n", args...)
	}
	transport := transports.NewMCPTransport(logger)

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

	// Choose the "hello" tool if available, otherwise pick the first one
	var toolName string
	for _, t := range tools {
		if strings.HasSuffix(t.Name, "hello") {
			toolName = t.Name
			break
		}
	}
	if toolName == "" {
		toolName = tools[0].Name
		fmt.Printf("WARNING: \"hello\" tool not found; defaulting to %s\n", toolName)
	}

	// Call the tool directly using MCPTransport
	argsMap := map[string]any{"count": "5"}
	stream, err := transport.CallToolStream(ctx, tools[1].Name, argsMap, mcpProvider)
	if err != nil {
		log.Fatalf("CallTool failed: %v", err)
	}
	for {
		msg, err := stream.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("stream error: %v", err)
		}
		fmt.Printf("Stream chunk: %#v\n", msg)
	}
	stream.Close()

	// Clean up - deregister the provider
	if err := transport.DeregisterToolProvider(ctx, mcpProvider); err != nil {
		log.Printf("Warning: failed to deregister provider: %v", err)
	}

	os.Exit(0)
}
