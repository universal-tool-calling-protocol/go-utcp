package main

import (
	"context"
	"fmt"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

func main() {
	ctx := context.Background()
	client, err := utcp.NewUTCPClient(ctx, &utcp.UtcpClientConfig{}, nil, nil)
	if err != nil {
		panic(err)
	}

	prov := utcp.NewMCPProvider("example")
	if _, err := client.RegisterToolProvider(ctx, prov); err != nil {
		panic(err)
	}

	res, err := client.CallTool(ctx, "demo", map[string]any{"param": "value"})
	if err != nil {
		fmt.Printf("call error: %v\n", err)
	} else {
		fmt.Printf("Result: %#v\n", res)
	}
}
