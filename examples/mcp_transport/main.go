package main

import (
	"context"
	"fmt"

	"github.com/Raezil/UTCP"
)

func main() {
	// Create a base context
	ctx := context.Background()

	// Initialize the MCP transport with the default logger
	transport := UTCP.NewMCPTransport(nil)

	// Create an MCPProvider instance with a name (use your own configuration)
	provider := UTCP.NewMCPProvider("ExampleProvider")

	// Register the provider with the transport

	tools, err := transport.RegisterToolProvider(ctx, provider)
	if err != nil {
		fmt.Printf("Error registering provider: %v\n", err)
		return
	}
	fmt.Printf("Registered provider '%s' with tools: %v\n", provider.Name(), tools)

	// Call a tool via the transport (replace "toolName" and parameters accordingly)
	result, err := transport.CallTool(ctx, "toolName", map[string]interface{}{"param1": "value1"}, provider, nil)
	if err != nil {
		fmt.Printf("Error invoking tool: %v\n", err)
	} else {
		fmt.Printf("Tool result: %v\n", result)
	}

	// Deregister the provider when done
	err = transport.DeregisterToolProvider(ctx, provider)
	if err != nil {
		fmt.Printf("Error deregistering provider: %v\n", err)
		return
	}
	fmt.Printf("Deregistered provider '%s' successfully\n", provider.Name())
}
