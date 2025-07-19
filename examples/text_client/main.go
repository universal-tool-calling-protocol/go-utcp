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

	tools, err := client.SearchTools(ctx, "", 10)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Discovered %d tools\n", len(tools))

	res, err := client.CallTool(ctx, "text.hello", map[string]any{"name": "World"})
	if err != nil {
		panic(err)
	}
	fmt.Printf("Result: %#v\n", res)
}
