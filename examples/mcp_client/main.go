package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	mcptransport "github.com/universal-tool-calling-protocol/go-utcp/src/transports/mcp"
)

func main() {
	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("client error: %v", err)
	}
	tools, err := client.SearchTools("", 10)
	fmt.Println("Tools were found:")
	for _, tool := range tools {
		fmt.Println("- ", tool.Name)
	}

	if err != nil {
		fmt.Errorf("Tools nof found")
	}
	args := map[string]any{
		"name": "Kamil",
	}
	data, err := client.CallTool(ctx, tools[0].Name, args)
	if err != nil {
		log.Fatalf("cannot proceed")

	}
	fmt.Println(data.(map[string]any)["result"])
	// 4) Synchronous call
	argsMap := map[string]any{"count": 5}
	result, err := client.CallTool(ctx, tools[1].Name, argsMap)
	if err != nil {
		log.Fatalf("cannot proceed")
	}

	if sub, ok := result.(*mcptransport.MCPSubscriptionResult); ok {
		for {
			val, err := sub.Next()
			if err != nil {
				if err == io.EOF {
					break
				}
				log.Fatalf("subscription next: %v", err)
			}
			fmt.Printf("Stream chunk: %#v\n", val)
		}
		if err := sub.Close(); err != nil {
			log.Fatalf("close error: %v", err)
		}
	} else {
		switch ev := result.(type) {
		case []interface{}:
			fmt.Println("Streamed tool response:")
			for i, chunk := range ev {
				fmt.Printf(" chunk %d: %#v\n", i+1, chunk)
			}
		default:
			fmt.Printf("Tool response: %#v\n", ev)
		}
	}
	os.Exit(0)
}
