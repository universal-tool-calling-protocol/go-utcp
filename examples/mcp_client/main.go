package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports/streamresult"
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
		fmt.Errorf("Tools nof found")
	}
	args := map[string]any{
		"name": "Kamil",
	}
	data, err := client.CallTool(ctx, tools[0].Name, args)
	if err != nil {
		log.Fatalf("cannot proceed")

	}
	if sr, ok := data.(*streamresult.SliceStreamResult); ok {
		val, err := sr.Next()
		if err != nil {
			log.Fatalf("stream next error: %v", err)
		}
		fmt.Println(val.(map[string]any)["result"])
	}
	// 4) Synchronous call
	argsMap := map[string]any{"count": 5}
	result, err := client.CallTool(ctx, tools[1].Name, argsMap)
	if err != nil {
		log.Fatalf("cannot proceed")
	}

	sr, ok := result.(*streamresult.SliceStreamResult)
	if !ok {
		log.Fatalf("unexpected result type %T", result)
	}
	fmt.Println("Streamed tool response:")
	for {
		chunk, err := sr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("next error: %v", err)
		}
		fmt.Printf(" chunk: %#v\n", chunk)
	}
	os.Exit(0)
}
