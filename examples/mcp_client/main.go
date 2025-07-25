package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
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

	// 4) Synchronous call
	argsMap := map[string]any{"count": 5}
	result, err := client.CallTool(ctx, tools[1].Name, argsMap)

	if err != nil {
		log.Fatalf("CallTool failed: %v", err)
	}

	if resMap, ok := result.(map[string]interface{}); ok {
		fmt.Println("Sync result:", resMap["result"])
	} else {
		fmt.Printf("Unexpected result type: %#v\n", result)
	}
	os.Exit(0)
}
