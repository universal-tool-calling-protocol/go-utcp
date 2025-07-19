package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

var upgrader = websocket.Upgrader{}

var tools = []utcp.Tool{{Name: "echo", Description: "Echoes a message"}}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	switch r.URL.Path {
	case "/tools":
		_, msg, err := c.ReadMessage()
		if err != nil || string(msg) != "manual" {
			return
		}
		manual := utcp.UtcpManual{Version: "1.0", Tools: tools}
		c.WriteJSON(manual)
	case "/echo":
		var in map[string]any
		if err := c.ReadJSON(&in); err == nil {
			c.WriteJSON(map[string]any{"result": in["msg"]})
		}
	}
}

func startServer(addr string) {
	http.HandleFunc("/tools", wsHandler)
	http.HandleFunc("/echo", wsHandler)
	log.Printf("WebSocket server on %s", addr)
	http.ListenAndServe(addr, nil)
}

func main() {
	go startServer(":8080")
	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("client: %v", err)
	}

	tools, err := client.SearchTools(ctx, "", 10)
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	log.Printf("discovered %d tools", len(tools))

	res, err := client.CallTool(ctx, "ws.echo", map[string]any{"msg": "hi"})
	if err != nil {
		log.Fatalf("call: %v", err)
	}
	log.Printf("Result: %#v", res)
}
