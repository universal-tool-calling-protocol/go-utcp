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
	time.Sleep(100 * time.Millisecond)
	res, err := client.CallTool(ctx, "greetings.hello", map[string]any{"name": "World"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "call error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Result: %v\n", res)
}
