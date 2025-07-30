package main

import (
	"context"
	"fmt"
	"log"
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
		fmt.Errorf("Tools not found")
	}
	args := map[string]any{
		"name": "Kamil",
	}
	data, err := client.CallTool(ctx, tools[0].Name, args, false)
	if err != nil {
		log.Fatalf("cannot proceed")
	}
	fmt.Println(data)
}
