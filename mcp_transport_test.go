package utcp

import (
	"context"
	"testing"
)

func TestMCPClientTransport_RegisterAndCall(t *testing.T) {
	tr := NewMCPTransport(nil)
	prov := NewMCPProvider("mcp")
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

	if res, err := tr.CallTool(ctx, "foo", nil, prov, nil); err != nil {
		t.Fatalf("call error: %v", err)
	} else if res == nil {
		t.Fatalf("expected non-nil result")
	}
}
