package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"

	UTCP "github.com/Raezil/UTCP"
)

func main() {
	ctx := context.Background()
	client, err := UTCP.NewUtcpClient(ctx, &UTCP.UtcpClientConfig{}, nil, nil)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	// Load and register the example provider
	data, err := ioutil.ReadFile("http_provider.json")
	if err != nil {
		log.Fatalf("read provider: %v", err)
	}
	var list []map[string]any
	if err := json.Unmarshal(data, &list); err != nil || len(list) == 0 {
		log.Fatalf("decode provider list: %v", err)
	}
	blob, _ := json.Marshal(list[0])
	prov, err := UTCP.UnmarshalProvider(blob)
	if err != nil {
		log.Fatalf("unmarshal provider: %v", err)
	}
	tools, err := client.RegisterToolProvider(ctx, prov)
	if err != nil {
		log.Fatalf("register provider: %v", err)
	}
	log.Printf("registered %d tool(s)", len(tools))

	// Invoke the first discovered tool if available
	if len(tools) > 0 {
		result, err := client.CallTool(ctx, tools[0].Name, map[string]any{})
		if err != nil {
			log.Fatalf("tool call failed: %v", err)
		}
		log.Printf("tool output: %v", result)
	}
}
