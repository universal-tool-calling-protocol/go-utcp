package websocket

import (
	"encoding/json"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/auth"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

// WebSocketProvider represents a WebSocket connection.
type WebSocketProvider struct {
	BaseProvider
	URL          string            `json:"url"`
	Protocol     *string           `json:"protocol,omitempty"`
	KeepAlive    bool              `json:"keep_alive"`
	Auth         *Auth             `json:"auth,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	HeaderFields []string          `json:"header_fields,omitempty"`
}

func UnmarshalWebSocketProvider(data []byte) (*WebSocketProvider, error) {
	type Alias WebSocketProvider
	aux := struct {
		*Alias
		Auth json.RawMessage `json:"auth"`
	}{Alias: (*Alias)(&WebSocketProvider{})}
	if err := json.Unmarshal(data, &aux); err != nil {
		return nil, err
	}
	wp := (*WebSocketProvider)(aux.Alias)
	if len(aux.Auth) > 0 {
		auth, err := UnmarshalAuth(aux.Auth)
		if err != nil {
			return nil, err
		}
		wp.Auth = &auth
	}
	return wp, nil
}
