package main

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/manual"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow connections from any origin
	},
}

// toolsHandler serves the UTCP manual/schema
func toolsHandler(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		return
	}
	defer c.Close()

	// Read the "manual" handshake message
	_, msg, err := c.ReadMessage()
	if err != nil || string(msg) != "manual" {
		log.Printf("expected 'manual' message, got: %s, err: %v", string(msg), err)
		return
	}

	// Define the supported tools
	manual := UtcpManual{
		Version: "1.0",
		Tools: []Tool{
			{
				Name:        "echo",
				Description: "Echo back the provided message",
			},
			{
				Name:        "multipleChunks",
				Description: "Send a response in multiple chunks",
			},
		},
	}

	if err := c.WriteJSON(manual); err != nil {
		log.Printf("error writing manual: %v", err)
	}
}

// echoHandler implements the 'echo' tool
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

	log.Printf("Received echo call: %#v", in)

	// Echo back the message
	response := map[string]any{
		"result": in["msg"],
	}

	if err := c.WriteJSON(response); err != nil {
		log.Printf("error writing response: %v", err)
	}
}

// multipleChunksHandler implements the 'multipleChunks' tool
func multipleChunksHandler(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		return
	}
	defer c.Close()

	// Read the tool call parameters (if any)
	var req map[string]any
	if err := c.ReadJSON(&req); err != nil {
		log.Printf("error reading JSON: %v", err)
		return
	}
	log.Printf("Received multipleChunks call: %#v", req)

	// Example chunks to send
	chunks := []string{"This", "is", "a", "response", "in", "multiple", "chunks"}
	for _, chunk := range chunks {
		// Send each chunk as its own message
		msg := map[string]any{"result": chunk}
		if err := c.WriteJSON(msg); err != nil {
			log.Printf("error writing chunk: %v", err)
			return
		}
		// Small delay to simulate streaming
		time.Sleep(100 * time.Millisecond)
	}
}

// startServer registers HTTP handlers and starts listening
func startServer(addr string) {
	http.HandleFunc("/tools", toolsHandler)
	http.HandleFunc("/websocket.echo", echoHandler)
	http.HandleFunc("/websocket.multipleChunks", multipleChunksHandler)
	log.Printf("WebSocket server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func main() {
	// Launch server
	go startServer(":8080")
	// Give server a moment to start
	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("client error: %v", err)
	}

	// Discover available tools
	tools, err := client.SearchTools("", 10)
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	log.Printf("Discovered tools:")
	for _, t := range tools {
		log.Printf(" - %s", t.Name)
	}

	// Call the streaming tool
	resIface, err := client.CallTool(ctx, "websocket.multipleChunks", map[string]any{})
	if err != nil {
		log.Fatalf("call error: %v", err)
	}

	// 1) Assert top‑level to slice
	chunks, ok := resIface.([]interface{})
	if !ok {
		log.Fatalf("unexpected response type: %T", resIface)
	}

	// 2) Loop & collect
	var combined strings.Builder
	for i, raw := range chunks {
		// 2a) Assert each element is a map
		m, ok := raw.(map[string]interface{})
		if !ok {
			log.Printf("chunk %d: unexpected type %T, skipping", i, raw)
			continue
		}

		// 2b) Pull the "result" field as string
		if piece, ok := m["result"].(string); ok {
			// You can do whatever you like with each piece!
			// E.g. append to a builder, print immediately, etc.
			combined.WriteString(piece)
			combined.WriteRune(' ')
		} else {
			log.Printf("chunk %d: missing or non‑string result field", i)
		}
	}

	// 3) Use the joined result
	log.Printf("Combined response: %q", combined.String())

}
