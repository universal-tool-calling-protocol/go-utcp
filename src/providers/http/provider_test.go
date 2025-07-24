package http

import (
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

func TestUnmarshalHttpProvider_Basic(t *testing.T) {
	jsonData := []byte(`{"provider_type":"http","name":"h","http_method":"POST","url":"http://example.com","content_type":"application/json"}`)
	p, err := UnmarshalHttpProvider(jsonData)
	if err != nil {
		t.Fatalf("unmarshal err: %v", err)
	}
	if p.Type() != ProviderHTTP {
		t.Fatalf("type mismatch")
	}
	if p.HTTPMethod != "POST" || p.URL != "http://example.com" {
		t.Fatalf("field mismatch")
	}
}
