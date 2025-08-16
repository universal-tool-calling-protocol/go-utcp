package grpc

import (
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

func TestUnmarshalGRPCProvider_Basic(t *testing.T) {
	jsonData := []byte(`{"provider_type":"grpc","name":"g","host":"localhost","port":5000,"service_name":"svc","method_name":"m","target":"router1"}`)
	p, err := UnmarshalGRPCProvider(jsonData)
	if err != nil {
		t.Fatalf("unmarshal err: %v", err)
	}
	if p.Type() != ProviderGRPC {
		t.Fatalf("type mismatch")
	}
	if p.Host != "localhost" || p.Port != 5000 || p.Target != "router1" {
		t.Fatalf("host, port or target mismatch: %#v", p)
	}
}
