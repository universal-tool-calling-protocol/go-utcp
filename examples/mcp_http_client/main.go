package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
)

func main() {
	// Allow MCP server to start up
	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("client error: %v", err)
	}

	// Discover tools
	tools, err := client.SearchTools("", 10)
	if err != nil {
		log.Fatalf("search error: %v", err)
	}
	fmt.Printf("Discovered %d tools:\n", len(tools))
	for _, t := range tools {
		fmt.Printf(" - %s\n", t.Name)
	}

	// Call hello tool
	res, err := client.CallTool(ctx, tools[0].Name, map[string]any{"name": "Go"})
	if err != nil {
		log.Fatalf("hello call error: %v", err)
	}
	fmt.Println(res)
	// Call streaming tool: returns StreamResult
	res, err = client.CallTool(ctx, tools[1].Name, map[string]any{
		"count":       5,
		"contentType": "event-stream",
	})
	if err != nil {
		log.Fatalf("stream call error: %v", err)
	}

	// Expect StreamResult
	sub, ok := res.(transports.StreamResult)
	if !ok {
		log.Fatalf("unexpected type: %T", res)
	}
	if err != nil {
		log.Fatalf("streaming call error: %v", err)
	}
	defer sub.Close()

	for {
		item, err := sub.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("stream next error: %v", err)
		}
		fmt.Println("Stream update:", item)
	}
}
