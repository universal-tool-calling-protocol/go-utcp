package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

var manual = utcp.UtcpManual{Version: "1.0", Tools: []utcp.Tool{
	{Name: "echo", Description: "Echo back a message"},
	{Name: "timestamp", Description: "Current time"},
}}

func startServer(addr string) {
	http.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.ContentLength == 0 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(manual)
			return
		}
		var req struct {
			Tool string                 `json:"tool"`
			Args map[string]interface{} `json:"args"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		switch req.Tool {
		case "echo":
			msg, _ := req.Args["message"].(string)
			json.NewEncoder(w).Encode(map[string]any{"result": msg})
		case "timestamp":
			json.NewEncoder(w).Encode(map[string]any{"result": time.Now().Format(time.RFC3339)})
		default:
			http.Error(w, "unknown tool", http.StatusNotFound)
		}
	})
	log.Printf("HTTP mock server on %s", addr)
	http.ListenAndServe(addr, nil)
}

func main() {
	go startServer(":8080")
	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("client error: %v", err)
	}

	tools, err := client.SearchTools("", 10)
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	fmt.Printf("Discovered %d tools:\n", len(tools))
	for _, t := range tools {
		fmt.Printf(" - %s\n", t.Name)
	}

	res, err := client.CallTool(ctx, "http.echo", map[string]any{"message": "hi"})
	if err != nil {
		log.Fatalf("call echo: %v", err)
	}
	fmt.Printf("Echo result: %#v\n", res)

	ts, err := client.CallTool(ctx, "http.timestamp", map[string]any{})
	if err != nil {
		log.Fatalf("call timestamp: %v", err)
	}
	fmt.Printf("Timestamp result: %#v\n", ts)
}
