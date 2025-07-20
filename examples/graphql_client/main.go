package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

func startServer(addr string) {
	http.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "only POST", http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}
		_ = json.Unmarshal(body, &req)
		if strings.Contains(req.Query, "__schema") {
			resp := map[string]any{"data": map[string]any{"__schema": map[string]any{"queryType": map[string]any{"fields": []map[string]any{{"name": "echo", "description": "Echo"}}}}}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if strings.Contains(req.Query, "echo") {
			msg, _ := req.Variables["msg"].(string)
			resp := map[string]any{"data": map[string]any{"echo": msg}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		http.Error(w, "unknown query", http.StatusBadRequest)
	})
	log.Printf("GraphQL server on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
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
	log.Printf("Discovered tools:")
	for _, t := range tools {
		log.Printf(" - %s", t.Name)
	}

	res, err := client.CallTool(ctx, "graphql.echo", map[string]any{"msg": "hi"})
	if err != nil {
		log.Fatalf("call: %v", err)
	}
	log.Printf("Result: %#v", res)
}
