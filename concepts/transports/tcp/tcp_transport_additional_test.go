package tcp

import (
	"context"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/concepts/providers"
)

func TestTCPTransport_DeregisterAndClose(t *testing.T) {
	tr := NewTCPClientTransport(nil)
	if err := tr.DeregisterToolProvider(context.Background(), &TCPProvider{}); err != nil {
		t.Fatalf("deregister error: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
}
