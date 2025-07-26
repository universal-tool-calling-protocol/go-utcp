package main

import (
	"context"
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

	// 5) Call hello tool
	result, err := client.CallTool(ctx, tools[0].Name, map[string]any{"name": "Go"})
	if err != nil {
		log.Fatalf("call error: %v", err)
	}
	fmt.Printf("Hello result: %#v\n", result)

	result, err = client.CallTool(ctx, tools[1].Name, map[string]any{"count": 5})
	if err != nil {
		log.Fatalf("call error: %v", err)
	}
	fmt.Printf("Hello result: %#v\n", result)
}
