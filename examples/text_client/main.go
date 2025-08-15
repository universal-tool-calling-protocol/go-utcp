package main

import (
	"context"
	"fmt"
	"os"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

func main() {
	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client error: %v\n", err)
		os.Exit(1)
	}
	tools, err := client.SearchTools("", 10)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client error %v\n", err)
	}

	time.Sleep(100 * time.Millisecond)
	res, err := client.CallTool(ctx, tools[0].Name, map[string]any{"name": "World"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "call error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Result: %v\n", res)
}
