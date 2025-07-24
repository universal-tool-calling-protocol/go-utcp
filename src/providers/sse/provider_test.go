package sse

import (
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

func TestUnmarshalSSEProvider_Basic(t *testing.T) {
	jsonData := []byte(`{"provider_type":"sse","name":"s","url":"http://events","reconnect":false,"retry_timeout":500}`)
	p, err := UnmarshalSSEProvider(jsonData)
	if err != nil {
		t.Fatalf("unmarshal err: %v", err)
	}
	if p.Type() != ProviderSSE {
		t.Fatalf("type mismatch")
	}
	if p.URL != "http://events" || p.Reconnect != false || p.RetryTimeout != 500 {
		t.Fatalf("field mismatch")
	}
}
