package main

import (
	"context"
	"fmt"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

func main() {
	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		panic(err)
	}

	tools, err := client.SearchTools("", 10)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Discovered %d tools\n", len(tools))
	for _, t := range tools {
		fmt.Printf(" - %s\n", t.Name)
	}

	res, err := client.CallTool(ctx, "text.hello", map[string]any{"name": "World"})
	if err != nil {
		panic(err)
	}
	fmt.Printf("Result: %#v\n", res)
}
