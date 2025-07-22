package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	src "github.com/universal-tool-calling-protocol/go-utcp/internal"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow connections from any origin
	},
}

func toolsHandler(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		return
	}
	defer c.Close()

	// Read the "manual" message
	_, msg, err := c.ReadMessage()
	if err != nil || string(msg) != "manual" {
		log.Printf("expected 'manual' message, got: %s, err: %v", string(msg), err)
		return
	}

	// Send the manual/schema
	manual := src.UtcpManual{
		Version: "1.0",
		Tools: []src.Tool{
			{
				Name:        "echo",
				Description: "Echo back the provided message",
			},
		},
	}

	if err := c.WriteJSON(manual); err != nil {
		log.Printf("error writing manual: %v", err)
	}
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		return
	}
	defer c.Close()

	// Read the tool call request
	var in map[string]any
	if err := c.ReadJSON(&in); err != nil {
		log.Printf("error reading JSON: %v", err)
		return
	}

	log.Printf("Received tool call: %#v", in)

	// Echo back the message
	response := map[string]any{
		"result": in["msg"],
	}

	if err := c.WriteJSON(response); err != nil {
		log.Printf("error writing response: %v", err)
	}
}

func startServer(addr string) {
	http.HandleFunc("/tools", toolsHandler)
	http.HandleFunc("/websocket.echo", echoHandler)
	log.Printf("WebSocket server listening on %s", addr)
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

	res, err := client.CallTool(ctx, "websocket.echo", map[string]any{"msg": "hello"})
	if err != nil {
		log.Fatalf("call error: %v", err)
	}
	log.Printf("Tool response: %#v", res)
}
