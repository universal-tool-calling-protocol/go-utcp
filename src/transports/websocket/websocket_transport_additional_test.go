package websocket

import (
	"context"
	"net/http"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/auth"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/http"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/websocket"
)

func TestWebSocketTransportApplyAuth_Basic(t *testing.T) {
	tr := NewWebSocketTransport(nil)
	var a Auth = NewBasicAuth("u", "p")
	prov := &WebSocketProvider{Auth: &a}
	hdr := http.Header{}
	if err := tr.applyAuth(hdr, prov); err != nil {
		t.Fatalf("applyAuth error: %v", err)
	}
	if hdr.Get("Authorization") == "" {
		t.Errorf("expected Authorization header set")
	}
}

func TestWebSocketTransportApplyAuth_Unsupported(t *testing.T) {
	tr := NewWebSocketTransport(nil)
	var dummyAuth Auth = &OAuth2Auth{TokenURL: "t", ClientID: "c", ClientSecret: "s"}
	prov := &WebSocketProvider{Auth: &dummyAuth}
	hdr := http.Header{}
	if err := tr.applyAuth(hdr, prov); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWebSocketTransport_RegisterWrongType(t *testing.T) {
	tr := NewWebSocketTransport(nil)
	_, err := tr.RegisterToolProvider(context.Background(), &HttpProvider{})
	if err == nil {
		t.Fatal("expected type error")
	}
}

func TestWebSocketTransport_CallWrongType(t *testing.T) {
	tr := NewWebSocketTransport(nil)
	_, err := tr.CallTool(context.Background(), "x", nil, &HttpProvider{}, false)
	if err == nil {
		t.Fatal("expected type error")
	}
}

func TestWebSocketTransport_DeregisterWrongType(t *testing.T) {
	tr := NewWebSocketTransport(nil)
	if err := tr.DeregisterToolProvider(context.Background(), &HttpProvider{}); err == nil {
		t.Fatal("expected type error")
	}
}
