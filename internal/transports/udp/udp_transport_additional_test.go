package udp

import (
	"context"

	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/internal/providers"
)

func TestUDPTransport_Deregister(t *testing.T) {
	tr := NewUDPTransport(nil)
	if err := tr.DeregisterToolProvider(context.Background(), &UDPProvider{}); err != nil {
		t.Fatalf("deregister error: %v", err)
	}
}
