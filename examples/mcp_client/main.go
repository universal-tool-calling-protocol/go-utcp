package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

func main() {
	// 1) Load provider.json
	data, err := ioutil.ReadFile("provider.json")
	if err != nil {
		log.Fatalf("Failed to read provider.json: %v", err)
	}

	// 2) Initialize MCPProvider
	provider, err := utcp.NewMCPProviderFromJSON(data)
	if err != nil {
		log.Fatalf("Invalid provider.json: %v", err)
	}

	// 3) Create transport and register
	transport := utcp.NewMCPTransport(func(format string, args ...interface{}) {
		fmt.Printf("[mcp] "+format+"\n", args...)
	})

	ctx := context.Background()

	tools, err := transport.RegisterToolProvider(ctx, provider)
	if err != nil {
		log.Fatalf("Register failed: %v", err)
	}

	fmt.Println("Discovered tools:")
	for _, t := range tools {
		fmt.Printf(" - %s: %s\n", t.Name, t.Description)
	}

	// 4) Synchronous call
	argsMap := map[string]any{"name": "Kamil"}
	result, err := transport.CallTool(ctx, "hello", argsMap, provider, nil)
	if err != nil {
		log.Fatalf("CallTool failed: %v", err)
	}

	if resMap, ok := result.(map[string]interface{}); ok {
		fmt.Println("Sync result:", resMap["result"])
	} else {
		fmt.Printf("Unexpected result type: %#v\n", result)
	}

	// 5) Streaming call demonstration
	stream, err := transport.CallToolStream(ctx, "call_stream", argsMap, provider)
	if err != nil {
		log.Fatalf("CallToolStream failed: %v", err)
	}

	fmt.Println("Streaming responses:")
	for msg := range stream {
		switch v := msg.(type) {
		case error:
			fmt.Println("Stream error:", v)
		default:
			// Expecting the same result structure
			if partMap, ok := v.(map[string]interface{}); ok {
				fmt.Println(partMap["result"])
			} else {
				fmt.Printf("Received: %#v\n", v)
			}
		}
	}

	// 6) Clean up
	if err := transport.DeregisterToolProvider(ctx, provider); err != nil {
		fmt.Printf("Warning: deregister error: %v\n", err)
	}
	os.Exit(0)
}
