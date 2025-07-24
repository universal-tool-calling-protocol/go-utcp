package websocket

import (
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

func TestUnmarshalWebSocketProvider_Basic(t *testing.T) {
	jsonData := []byte(`{"provider_type":"websocket","name":"ws","url":"ws://x","keep_alive":true}`)
	p, err := UnmarshalWebSocketProvider(jsonData)
	if err != nil {
		t.Fatalf("unmarshal err: %v", err)
	}
	if p.Type() != ProviderWebSocket {
		t.Fatalf("type mismatch")
	}
	if p.URL != "ws://x" || !p.KeepAlive {
		t.Fatalf("field mismatch")
	}
}
