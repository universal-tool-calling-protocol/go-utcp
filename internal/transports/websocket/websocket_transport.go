package websocket

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	. "github.com/universal-tool-calling-protocol/go-utcp/internal/providers"

	. "github.com/universal-tool-calling-protocol/go-utcp/internal/auth"
	. "github.com/universal-tool-calling-protocol/go-utcp/internal/concepts"

	"time"

	"github.com/gorilla/websocket"
)

type WebSocketClientTransport struct {
	dialer *websocket.Dialer
	logger func(format string, args ...interface{})
}

func NewWebSocketTransport(logger func(format string, args ...interface{})) *WebSocketClientTransport {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}
	return &WebSocketClientTransport{
		dialer: &websocket.Dialer{HandshakeTimeout: 30 * time.Second},
		logger: logger,
	}
}

func (t *WebSocketClientTransport) applyAuth(h http.Header, prov *WebSocketProvider) error {
	if prov.Auth == nil {
		return nil
	}
	authIfc := *prov.Auth
	switch a := authIfc.(type) {
	case *ApiKeyAuth:
		if strings.ToLower(a.Location) == "header" {
			h.Set(a.VarName, a.APIKey)
		}
	case *BasicAuth:
		enc := base64.StdEncoding.EncodeToString([]byte(a.Username + ":" + a.Password))
		h.Set("Authorization", "Basic "+enc)
	default:
		return errors.New("unsupported auth type")
	}
	return nil
}

func (t *WebSocketClientTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	wsProv, ok := prov.(*WebSocketProvider)
	if !ok {
		return nil, errors.New("WebSocketClientTransport can only be used with WebSocketProvider")
	}

	hdr := http.Header{}
	for k, v := range wsProv.Headers {
		hdr.Set(k, v)
	}
	if err := t.applyAuth(hdr, wsProv); err != nil {
		return nil, err
	}
	if wsProv.Protocol != nil {
		hdr.Set("Sec-WebSocket-Protocol", *wsProv.Protocol)
	}

	conn, _, err := t.dialer.DialContext(ctx, wsProv.URL, hdr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte("manual")); err != nil {
		return nil, err
	}
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	var manual UtcpManual
	if err := json.Unmarshal(msg, &manual); err != nil {
		return nil, err
	}
	return manual.Tools, nil
}

func (t *WebSocketClientTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	_, ok := prov.(*WebSocketProvider)
	if !ok {
		return errors.New("WebSocketClientTransport can only be used with WebSocketProvider")
	}
	return nil
}

func (t *WebSocketClientTransport) CallTool(ctx context.Context, toolName string, args map[string]any, prov Provider, l *string) (any, error) {
	wsProv, ok := prov.(*WebSocketProvider)
	if !ok {
		return nil, errors.New("WebSocketClientTransport can only be used with WebSocketProvider")
	}

	hdr := http.Header{}
	for k, v := range wsProv.Headers {
		hdr.Set(k, v)
	}
	if err := t.applyAuth(hdr, wsProv); err != nil {
		return nil, err
	}
	if wsProv.Protocol != nil {
		hdr.Set("Sec-WebSocket-Protocol", *wsProv.Protocol)
	}

	url := strings.TrimSuffix(wsProv.URL, "/tools")
	if !strings.HasSuffix(url, "/"+toolName) {
		if strings.HasSuffix(url, "/") {
			url = strings.TrimSuffix(url, "/")
		}
		url = url + "/" + toolName
	}

	conn, _, err := t.dialer.DialContext(ctx, url, hdr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	data, _ := json.Marshal(args)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return nil, err
	}
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	var result any
	if err := json.Unmarshal(msg, &result); err != nil {
		return nil, err
	}
	return result, nil
}
