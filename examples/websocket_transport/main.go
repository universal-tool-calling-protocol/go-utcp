package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/manual"
	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/websocket"
	transports "github.com/universal-tool-calling-protocol/go-utcp/src/transports/websocket"
)

var upgrader = websocket.Upgrader{}

var tools = []Tool{
	{Name: "echo", Description: "Echoes a message"},
}

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
		manual := UtcpManual{Version: "1.0", Tools: tools}
		c.WriteJSON(manual)
	case "/echo":
		var in map[string]any
		if err := c.ReadJSON(&in); err != nil {
			return
		}
		c.WriteJSON(map[string]any{"result": in["msg"]})
	}
}

func startServer(addr string) {
	http.HandleFunc("/tools", wsHandler)
	http.HandleFunc("/echo", wsHandler)
	log.Printf("WebSocket server listening on %s", addr)
	http.ListenAndServe(addr, nil)
}

func main() {
	go startServer(":8080")
	time.Sleep(200 * time.Millisecond)

	logger := func(format string, args ...interface{}) { log.Printf(format, args...) }
	transport := transports.NewWebSocketTransport(logger)
	wsURL := "ws://localhost:8080/tools"
	prov := &providers.WebSocketProvider{BaseProvider: BaseProvider{Name: "ws", ProviderType: ProviderWebSocket}, URL: wsURL}

	ctx := context.Background()
	discovered, err := transport.RegisterToolProvider(ctx, prov)
	if err != nil {
		log.Fatalf("register error: %v", err)
	}
	log.Printf("Discovered tools via websocket:")
	for _, t := range discovered {
		log.Printf(" - %s: %s", t.Name, t.Description)
	}

	res, err := transport.CallTool(ctx, "echo", map[string]any{"msg": "hello"}, prov)
	if err != nil {
		log.Fatalf("call error: %v", err)
	}
	log.Printf("Tool response: %#v", res)

	_ = transport.DeregisterToolProvider(ctx, prov)
}
