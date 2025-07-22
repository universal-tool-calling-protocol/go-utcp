package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

func main() {
	// Give providers a moment to start
	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("failed to create UTCP client: %v", err)
	}

	// 1) Discover available tools
	tools, err := client.SearchTools("", 10)
	if err != nil {
		log.Fatalf("SearchTools failed: %v", err)
	}
	if len(tools) == 0 {
		log.Fatal("no tools found")
	}

	fmt.Println("Tools were found:")
	for _, t := range tools {
		fmt.Printf(" - %s\n", t.Name)
	}

	// 2) Choose the "hello" tool if available, otherwise pick the first one
	var toolName string
	for _, t := range tools {
		if strings.HasSuffix(t.Name, ".hello") {
			toolName = t.Name
			break
		}
	}
	if toolName == "" {
		toolName = tools[0].Name
		fmt.Printf("WARNING: \".hello\" not found; defaulting to %s\n", toolName)
	}

	// 3) Call the tool synchronously
	argsMap := map[string]any{"name": "Kamil"}
	result, err := client.CallTool(ctx, toolName, argsMap)
	if err != nil {
		log.Fatalf("CallTool failed: %v", err)
	}

	// 4) Print the response
	if resMap, ok := result.(map[string]interface{}); ok {
		fmt.Println("Sync result:", resMap["result"])
	} else {
		fmt.Printf("Unexpected result type: %#v\n", result)
	}

	os.Exit(0)
}
