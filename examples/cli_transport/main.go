package main

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/transports/cli"
)

func main() {
	ctx := context.Background()

	// Create a logger function
	logger := func(format string, args ...interface{}) {
		fmt.Printf("[LOG] "+format+"\n", args...)
	}

	// Create the CLI transport directly
	fmt.Println("Creating CLI transport...")
	transport := NewCliTransport(logger)

	// Create a CLI provider configuration
	// This should match your discover_hello.sh script
	provider := &CliProvider{
		CommandName: "./discover_hello.sh discover", // Command to discover tools
		EnvVars:     map[string]string{},            // Any environment variables needed
		WorkingDir:  nil,                            // Working directory (nil for current)
	}

	// Give some time for setup
	fmt.Println("Waiting for initialization...")
	time.Sleep(500 * time.Millisecond)

	fmt.Println("\n=== Tool Discovery ===")

	// Register the tool provider and discover tools
	tools, err := transport.RegisterToolProvider(ctx, provider)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to register provider: %v\n", err)
		os.Exit(1)
	}

	if len(tools) == 0 {
		fmt.Println("No tools found!")
		os.Exit(1)
	}

	tool := tools[0]
	fmt.Printf("Found tool: %s\n", tool.Name)
	fmt.Printf("Tool description: %s\n", tool.Description)

	// Test the tool call
	fmt.Println("\n=== Tool Call Test ===")
	input := map[string]interface{}{
		"name": "Kamil",
	}

	fmt.Printf("Calling tool '%s' with input: %v\n", tool.Name, input)

	// Call the tool directly using the transport
	result, err := transport.CallTool(ctx, tool.Name, input, provider, nil)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		fmt.Printf("Error type: %T\n", err)
		fmt.Printf("Error string: %s\n", err.Error())
	} else {
		fmt.Printf("SUCCESS: %v\n", result)
	}

	// Clean up
	transport.Close()
}
