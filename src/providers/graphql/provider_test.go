package graphql

import (
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

func TestUnmarshalGraphQLProvider_Basic(t *testing.T) {
	jsonData := []byte(`{"provider_type":"graphql","name":"g","url":"http://x","operation_type":"query"}`)
	p, err := UnmarshalGraphQLProvider(jsonData)
	if err != nil {
		t.Fatalf("unmarshal err: %v", err)
	}
	if p.Type() != ProviderGraphQL {
		t.Fatalf("type mismatch")
	}
	if p.URL != "http://x" {
		t.Fatalf("url mismatch")
	}
	if p.OperationType != "query" {
		t.Fatalf("operation type mismatch")
	}
}
