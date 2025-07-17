package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Raezil/UTCP"
)

func main() {
	// Create a simple logger
	logger := func(format string, args ...interface{}) {
		log.Printf(format, args...)
	}

	// Initialize HTTP transport
	transport := UTCP.NewHttpClientTransport(logger)

	// Define an example HTTP provider
	provider := &UTCP.HttpProvider{
		URL:        "https://api.example.com/tools",
		HTTPMethod: "GET",
		Headers: map[string]string{
			"Accept": "application/json",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Discover available tools

	tools, err := transport.RegisterToolProvider(ctx, provider)
	if err != nil {
		log.Fatalf("Failed to register provider: %v", err)
	}

	// List discovered tools
	fmt.Println("Discovered tools:")
	for _, t := range tools {
		fmt.Printf("- %s: %s\n", t.Name, t.Description)
	}

	// Example: call a tool named "echo"
	args := map[string]interface{}{"message": "Hello from Go!"}
	var logOutput *string
	result, err := transport.CallTool(ctx, "echo", args, provider, logOutput)
	if err != nil {
		log.Fatalf("Tool call failed: %v", err)
	}

	// Print the result
	fmt.Printf("Tool response: %v\n", result)
}
