package http

import (
	"encoding/json"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/auth"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

// HttpProvider represents RESTful HTTP/HTTPS API.
type HttpProvider struct {
	BaseProvider
	HTTPMethod   string            `json:"http_method"` // GET, POST, PUT, DELETE, PATCH
	URL          string            `json:"url"`
	ContentType  string            `json:"content_type"` // default application/json
	Auth         *Auth             `json:"auth,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	BodyField    *string           `json:"body_field,omitempty"` // name of the single input field
	HeaderFields []string          `json:"header_fields,omitempty"`
}

func UnmarshalHttpProvider(data []byte) (*HttpProvider, error) {
	type Alias HttpProvider
	aux := struct {
		*Alias
		Auth json.RawMessage `json:"auth"`
	}{Alias: (*Alias)(&HttpProvider{})}
	if err := json.Unmarshal(data, &aux); err != nil {
		return nil, err
	}
	hp := (*HttpProvider)(aux.Alias)
	if len(aux.Auth) > 0 {
		auth, err := UnmarshalAuth(aux.Auth)
		if err != nil {
			return nil, err
		}
		hp.Auth = &auth
	}
	return hp, nil
}
