package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

func main() {
	ctx := context.Background()
	_, file, _, _ := runtime.Caller(0)
	base := filepath.Dir(file)
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: filepath.Join(base, "provider.json")}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client error: %v\n", err)
		os.Exit(1)
	}
	tools, err := client.SearchTools("", 10)
	if err != nil || len(tools) == 0 {
		fmt.Fprintf(os.Stderr, "no tools found: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Calling tool %s\n", tools[0].Name)
	res, err := client.CallTool(ctx, tools[0].Name, map[string]any{"name": "World"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "call error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Result: %v\n", res)
}
