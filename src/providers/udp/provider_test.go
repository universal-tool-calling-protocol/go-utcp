package udp

import (
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

func TestUDPProvider_Basic(t *testing.T) {
	p := &UDPProvider{
		BaseProvider: BaseProvider{Name: "udp", ProviderType: ProviderUDP},
		Host:         "localhost",
		Port:         9090,
		Timeout:      200,
	}
	if p.Type() != ProviderUDP {
		t.Fatalf("type mismatch")
	}
	if p.Port != 9090 {
		t.Fatalf("port mismatch")
	}
}
