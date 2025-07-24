package streamable

import (
	"encoding/json"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/auth"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

type StreamableHttpProvider struct {
	BaseProvider
	URL          string            `json:"url"`
	HTTPMethod   string            `json:"http_method"`  // GET, POST
	ContentType  string            `json:"content_type"` // default application/octet-stream
	ChunkSize    int               `json:"chunk_size"`   // bytes, default 4096
	Timeout      int               `json:"timeout"`      // ms, default 60000
	Headers      map[string]string `json:"headers,omitempty"`
	Auth         *Auth             `json:"auth,omitempty"`
	BodyField    *string           `json:"body_field,omitempty"`
	HeaderFields []string          `json:"header_fields,omitempty"`
}

func UnmarshalStreamableHttpProvider(data []byte) (*StreamableHttpProvider, error) {
	type Alias StreamableHttpProvider
	aux := struct {
		*Alias
		Auth json.RawMessage `json:"auth"`
	}{Alias: (*Alias)(&StreamableHttpProvider{})}
	if err := json.Unmarshal(data, &aux); err != nil {
		return nil, err
	}
	sp := (*StreamableHttpProvider)(aux.Alias)
	if len(aux.Auth) > 0 {
		auth, err := UnmarshalAuth(aux.Auth)
		if err != nil {
			return nil, err
		}
		sp.Auth = &auth
	}
	return sp, nil
}
