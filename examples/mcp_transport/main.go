package main

import (
	"context"
	"fmt"

	"github.com/universal-tool-calling-protocol/go-utcp"
)

func main() {
	ctx := context.Background()

	// Initialize the MCP transport with a simple logger
	transport := utcp.NewMCPTransport(func(format string, args ...interface{}) {
		fmt.Printf("[MCP] "+format+"\n", args...)
	})

	// Configure your provider to launch the Python MCP server
	provider := utcp.NewMCPProvider(
		"ExampleProvider",
		[]string{"python3", "server.py"},
	)

	// Register the provider and retrieve the available tools
	tools, err := transport.RegisterToolProvider(ctx, provider)
	if err != nil {
		fmt.Printf("Error registering provider: %v\n", err)
		return
	}
	fmt.Printf("Registered provider '%s' with tools: %v\n", provider.Name, tools)

	// Choose a tool to call (for example, the first one)
	if len(tools) == 0 {
		fmt.Println("No tools available to call.")
		return
	}
	toolID := tools[0] // or pick by name: tools.Find("tool_name")

	// Prepare arguments for the tool call
	// Replace the slice below with the actual arguments your tool expects
	args := map[string]any{"name": "Kamil"}

	// Call the tool and handle the response
	resp, err := transport.CallTool(ctx, toolID.Name, args, provider, nil)
	if err != nil {
		fmt.Printf("Error calling tool '%s': %v\n", toolID, err)
		return
	}

	// Process the result (resp.Result is typically a JSON-decoded value)
	fmt.Printf("Tool '%s' returned: %v\n", toolID, resp)
}
