package main

import (
	"context"
	"encoding/json"
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

	subRes, ok := res.(transports.StreamResult)
	if !ok {
		return
	}
	for {
		item, err := subRes.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("stream next error: %v", err)
		}

		// If payload is a notification with JSON array, split it
		if m, ok := item.(map[string]any); ok && m["method"] == "streamChunk" {
			params := m["params"].(map[string]any)
			if textRaw, ok := params["text"].(string); ok {
				var parts []string
				if err := json.Unmarshal([]byte(textRaw), &parts); err == nil {
					for _, p := range parts {
						fmt.Println(p)
					}
					continue
				}
			}
			// Fallback: print raw
			fmt.Printf("%#v\n", item)
			continue
		}

		// Otherwise print the item
		fmt.Printf("Stream update: %#v\n", item)
	}
	// Call streaming tool: returns StreamResult
	res, err = client.CallTool(ctx, tools[1].Name, map[string]any{"count": 5})
	if err != nil {
		log.Fatalf("stream call error: %v", err)
	}

	// Expect StreamResult
	sub, ok := res.(transports.StreamResult)
	if !ok {
		log.Fatalf("unexpected type: %T", subRes)
	}
	defer sub.Close()

	// Iterate over stream
	for {
		item, err := sub.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("stream next error: %v", err)
		}

		// If payload is a notification with JSON array, split it
		if m, ok := item.(map[string]any); ok && m["method"] == "streamChunk" {
			params := m["params"].(map[string]any)
			if textRaw, ok := params["text"].(string); ok {
				var parts []string
				if err := json.Unmarshal([]byte(textRaw), &parts); err == nil {
					for _, p := range parts {
						fmt.Println(p)
					}
					continue
				}
			}
			// Fallback: print raw
			fmt.Printf("%#v\n", item)
			continue
		}

		// Otherwise print the item
		fmt.Printf("Stream update: %#v\n", item)
	}
}
