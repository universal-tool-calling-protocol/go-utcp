package streamable

import (
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

func TestUnmarshalStreamableHttpProvider_Basic(t *testing.T) {
	jsonData := []byte(`{"provider_type":"http_stream","name":"s","url":"http://x","http_method":"GET"}`)
	p, err := UnmarshalStreamableHttpProvider(jsonData)
	if err != nil {
		t.Fatalf("unmarshal err: %v", err)
	}
	if p.Type() != ProviderHTTPStream {
		t.Fatalf("type mismatch")
	}
	if p.URL != "http://x" || p.HTTPMethod != "GET" {
		t.Fatalf("field mismatch")
	}
}
