package main

import (
	"context"
	"fmt"
	"log"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

func main() {
	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("failed to create CodeMode client: %v", err)
	}

	chain := []utcp.ChainStep{
		{
			ToolName: "demo_tools.hello",
			Inputs:   map[string]any{"name": "Go Mode"},
		},
		{
			ToolName:    "demo_tools.process",
			UsePrevious: true, // feed output from previous step
		},
	}

	result, err := client.CallToolChain(ctx, chain, 20*time.Second)
	if err != nil {
		log.Fatalf("chain execution failed: %v", err)
	}

	fmt.Println("=== Chain Results ===")
	for name, out := range result {
		fmt.Printf("%s → %+v\n", name, out)
	}
}
