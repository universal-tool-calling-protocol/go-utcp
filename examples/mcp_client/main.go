package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	transports "github.com/universal-tool-calling-protocol/go-utcp/src/transports"
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
	data, err := client.CallTool(ctx, tools[0].Name, args)
	if err != nil {
		log.Fatalf("cannot proceed")
	}
	fmt.Println(data.(map[string]any)["result"])

	// 4) Synchronous call
	argsMap := map[string]any{"count": 5}
	res, err := client.CallTool(ctx, tools[1].Name, argsMap)
	if err != nil {
		log.Fatalf("subscription call error: %v", err)
	}
	sub, ok := res.(*transports.ChannelStreamResult)
	if !ok {
		log.Fatalf("unexpected subscription type: %T", res)
	}
	for {
		val, err := sub.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("subscription next error: %v", err)
		}
		fmt.Printf("Subscription update: %#v\n", val)
	}
	sub.Close()
}
