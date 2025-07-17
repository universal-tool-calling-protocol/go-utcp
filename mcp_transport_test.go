package UTCP

import (
	"context"
	"testing"
)

func TestMCPClientTransport_RegisterAndCall(t *testing.T) {
	tr := NewMCPTransport(nil)
	prov := &MCPProvider{BaseProvider: BaseProvider{Name: "mcp", ProviderType: ProviderMCP}, Config: McpConfig{}}
	ctx := context.Background()
	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if tools != nil {
		t.Fatalf("expected nil tools, got %v", tools)
	}

	if err := tr.DeregisterToolProvider(ctx, prov); err != nil {
		t.Fatalf("deregister error: %v", err)
	}

	if _, err := tr.CallTool(ctx, "foo", nil, prov, nil); err == nil {
		t.Fatalf("expected call error")
	}
}
