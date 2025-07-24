package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	mcpprov "github.com/universal-tool-calling-protocol/go-utcp/src/providers/mcp"
	utcp "github.com/universal-tool-calling-protocol/go-utcp/src/transports/mcp"
)

func main() {
	// launch FastMCP demo server
	cmd := exec.Command("python3", "../../scripts/fastmcp_server.py")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Fatalf("failed to start fastmcp server: %v", err)
	}
	defer cmd.Process.Kill()

	// give server time
	time.Sleep(2 * time.Second)

	ctx := context.Background()
	transport := utcp.NewMCPServerTransport(func(format string, args ...interface{}) {
		fmt.Printf("[MCP-HTTP] "+format+"\n", args...)
	})

	provider := mcpprov.NewMCPServerProvider("http://127.0.0.1:8008/mcp")

	tools, err := transport.RegisterToolProvider(ctx, provider)
	if err != nil {
		log.Fatalf("register error: %v", err)
	}
	fmt.Printf("discovered %d tools\n", len(tools))
	if len(tools) == 0 {
		return
	}

	result, err := transport.CallTool(ctx, tools[0].Name, map[string]any{"name": "Go"}, provider, nil)
	if err != nil {
		log.Fatalf("call error: %v", err)
	}
	fmt.Printf("result: %#v\n", result)
}
