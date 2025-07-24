package sse

import (
	"encoding/json"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/auth"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

// SSEProvider represents Server-Sent Events.
type SSEProvider struct {
	BaseProvider
	URL          string            `json:"url"`
	EventType    *string           `json:"event_type,omitempty"`
	Reconnect    bool              `json:"reconnect"`     // default true
	RetryTimeout int               `json:"retry_timeout"` // ms, default 30000
	Auth         *Auth             `json:"auth,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	BodyField    *string           `json:"body_field,omitempty"`
	HeaderFields []string          `json:"header_fields,omitempty"`
}

func UnmarshalSSEProvider(data []byte) (*SSEProvider, error) {
	type Alias SSEProvider
	aux := struct {
		*Alias
		Auth json.RawMessage `json:"auth"`
	}{Alias: (*Alias)(&SSEProvider{})}
	if err := json.Unmarshal(data, &aux); err != nil {
		return nil, err
	}
	sp := (*SSEProvider)(aux.Alias)
	if len(aux.Auth) > 0 {
		auth, err := UnmarshalAuth(aux.Auth)
		if err != nil {
			return nil, err
		}
		sp.Auth = &auth
	}
	return sp, nil
}
