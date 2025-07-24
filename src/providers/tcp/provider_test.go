package tcp

import (
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

func TestTCPProvider_Basic(t *testing.T) {
	p := &TCPProvider{
		BaseProvider: BaseProvider{Name: "tcp", ProviderType: ProviderTCP},
		Host:         "localhost",
		Port:         8080,
		Timeout:      100,
	}
	if p.Type() != ProviderTCP {
		t.Fatalf("type mismatch")
	}
	if p.Port != 8080 {
		t.Fatalf("port mismatch")
	}
}
