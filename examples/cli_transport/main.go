package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Raezil/UTCP"
)

func main() {
	ctx := context.Background()

	cfg := &UTCP.UtcpClientConfig{
		ProvidersFilePath: "providers.json",
	}

	fmt.Println("Creating UTCP client...")
	client, err := UTCP.NewUtcpClient(ctx, cfg, nil, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create UTCP client: %v\n", err)
		os.Exit(1)
	}

	// Give the client time to fully initialize
	fmt.Println("Waiting for initialization...")
	time.Sleep(500 * time.Millisecond)

	fmt.Println("\n=== Tool Discovery ===")
	tools, err := client.SearchTools("", 10)
	if err != nil {
		fmt.Fprintf(os.Stderr, "search error: %v\n", err)
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
	result, err := client.CallTool(ctx, tool.Name, input)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)

		// Try to understand the error better
		fmt.Printf("Error type: %T\n", err)
		fmt.Printf("Error string: %s\n", err.Error())

		// Let's try a direct search for the provider
		fmt.Println("\n=== Searching for provider directly ===")
		providerTools, err2 := client.SearchTools("hello", 10)
		if err2 != nil {
			fmt.Printf("Provider search failed: %v\n", err2)
		} else {
			fmt.Printf("Provider search returned %d tools\n", len(providerTools))
			for i, t := range providerTools {
				fmt.Printf("  %d: %s\n", i, t.Name)
			}
		}

	} else {
		fmt.Printf("SUCCESS: %v\n", result)
	}
}
