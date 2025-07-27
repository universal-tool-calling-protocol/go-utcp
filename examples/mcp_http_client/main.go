package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/universal-tool-calling-protocol/go-utcp"
)

func main() {
	// 1) Start the MCP HTTP server
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

	// Call hello tool
	result, err := client.CallTool(ctx, tools[0].Name, map[string]any{"name": "Go"})
	if err != nil {
		log.Fatalf("call error: %v", err)
	}
	fmt.Printf("Hello result: %#v\n", result)

	// Call the streaming tool
	result, err = client.CallTool(ctx, "demo_tools.handle_call_stream", map[string]any{"count": 5})
	if err != nil {
		log.Fatalf("call error: %v", err)
	}

	// Normalize to a slice of messages
	var chunks []any
	switch v := result.(type) {
	case []any:
		chunks = v
	case map[string]any:
		chunks = []any{v}
	default:
		log.Fatalf("unexpected result type: %T", result)
	}

	// Process each notification
	for _, raw := range chunks {
		payload, ok := raw.(map[string]any)
		if !ok {
			fmt.Printf("unexpected chunk type: %#v\n", raw)
			continue
		}

		// Only streamChunk messages
		if payload["method"] != "streamChunk" {
			continue
		}

		params, ok := payload["params"].(map[string]any)
		if !ok {
			continue
		}

		// Extract the text field
		textVal, ok := params["text"].(string)
		if !ok {
			continue
		}

		// Try unmarshalling into []string
		var items []string
		if err := json.Unmarshal([]byte(textVal), &items); err != nil {
			// Not a JSON array: print raw
			fmt.Println("Chunk:", textVal)
			continue
		}

		// Print each sub-chunk
		for i, item := range items {
			fmt.Printf("Chunk %d: %s\n", i+1, item)
		}
	}
}
