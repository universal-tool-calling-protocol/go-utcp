package graphql

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/graphql"

	"github.com/gorilla/websocket"
)

func TestGraphQLClientTransport_WebSocketSubscription(t *testing.T) {
	upgrader := websocket.Upgrader{Subprotocols: []string{"graphql-ws"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade error: %v", err)
		}
		defer c.Close()
		if r.Header.Get("Sec-WebSocket-Protocol") != "graphql-ws" {
			t.Errorf("expected subprotocol header graphql-ws, got %s", r.Header.Get("Sec-WebSocket-Protocol"))
		}
		var msg map[string]any
		if err := c.ReadJSON(&msg); err != nil {
			t.Fatalf("read init: %v", err)
		}
		if msg["type"] != "connection_init" {
			t.Fatalf("expected connection_init, got %v", msg["type"])
		}
		c.WriteJSON(map[string]any{"type": "connection_ack"})
		if err := c.ReadJSON(&msg); err != nil {
			t.Fatalf("read start: %v", err)
		}
		if msg["type"] != "start" {
			t.Fatalf("expected start, got %v", msg["type"])
		}
		c.WriteJSON(map[string]any{
			"type": "data",
			"payload": map[string]any{
				"data": map[string]any{"updates": 42},
			},
		})
		c.WriteJSON(map[string]any{"type": "complete"})
	}))
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	prov := &GraphQLProvider{BaseProvider: BaseProvider{ProviderType: ProviderGraphQL}, URL: wsURL, OperationType: "subscription"}
	tr := NewGraphQLClientTransport(nil)
	res, err := tr.CallTool(context.Background(), "updates", nil, prov, false)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	sub, ok := res.(*SubscriptionResult)
	if !ok {
		t.Fatalf("expected SubscriptionResult, got %T", res)
	}
	val, err := sub.Next()
	if err != nil {
		t.Fatalf("next error: %v", err)
	}
	if val.(float64) != 42 {
		t.Fatalf("unexpected value: %v", val)
	}
	if err := sub.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
	if _, err = sub.Next(); err == nil {
		t.Fatalf("expected EOF after close")
	}
}
