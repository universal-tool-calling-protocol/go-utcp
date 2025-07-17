package UTCP

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestWebSocketTransport_RegisterAndCall(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			c.WriteJSON(map[string]any{
				"version": "1.0",
				"tools":   []map[string]any{{"name": "ping", "description": "Ping"}},
			})
		case "/ping":
			var in map[string]any
			if err := c.ReadJSON(&in); err != nil {
				return
			}
			c.WriteJSON(map[string]any{"pong": in["msg"]})
		}
	}))
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	prov := &WebSocketProvider{
		BaseProvider: BaseProvider{Name: "ws", ProviderType: ProviderWebSocket},
		URL:          wsURL + "/tools",
	}
	tr := NewWebSocketTransport(nil)
	ctx := context.Background()

	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "ping" {
		t.Fatalf("unexpected tools: %+v", tools)
	}

	prov.URL = wsURL
	res, err := tr.CallTool(ctx, "ping", map[string]any{"msg": "hi"}, prov, nil)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok || m["pong"] != "hi" {
		t.Fatalf("unexpected result: %#v", res)
	}
}
