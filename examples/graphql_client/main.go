package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	graphqltransport "github.com/universal-tool-calling-protocol/go-utcp/src/transports/graphql"
)

func startServer(addr string) {
	upgrader := websocket.Upgrader{Subprotocols: []string{"graphql-ws"}}

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

	// Simple WebSocket subscription endpoint
	http.HandleFunc("/sub", func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("upgrade error: %v", err)
			return
		}
		defer c.Close()

		// Expect connection_init
		var msg map[string]any
		if err := c.ReadJSON(&msg); err != nil {
			log.Printf("read init: %v", err)
			return
		}
		c.WriteJSON(map[string]any{"type": "connection_ack"})
		if err := c.ReadJSON(&msg); err != nil {
			log.Printf("read start: %v", err)
			return
		}
		// Send a couple of updates
		for i := 1; i <= 2; i++ {
			c.WriteJSON(map[string]any{
				"type": "data",
				"payload": map[string]any{
					"data": map[string]any{"updates": i},
				},
			})
			time.Sleep(100 * time.Millisecond)
		}
		c.WriteJSON(map[string]any{"type": "complete"})
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

	// ---- Subscription example ----
	subRes, err := client.CallTool(ctx, "graphql_sub.updates", nil)
	if err != nil {
		log.Fatalf("subscription call: %v", err)
	}
	sub, ok := subRes.(*graphqltransport.SubscriptionResult)
	if !ok {
		log.Fatalf("unexpected subscription type: %T", subRes)
	}
	for {
		val, err := sub.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("subscription next: %v", err)
		}
		log.Printf("Update: %#v", val)
	}
	if err := sub.Close(); err != nil {
		log.Fatalf("close error: %v", err)
	}
}
