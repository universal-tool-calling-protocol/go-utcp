package mcp

import (
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

func TestMCPProvider_Basic(t *testing.T) {
	p := NewMCPProvider("n", []string{"/home/raezil/go-utcp/examples/mcp_client/mcp_server"})
	if p.Type() != ProviderType("mcp") {
		t.Fatalf("Type mismatch")
	}
	if p.Name != "n" {
		t.Fatalf("Name mismatch")
	}
}
