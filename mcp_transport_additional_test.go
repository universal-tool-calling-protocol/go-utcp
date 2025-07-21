package utcp

import (
	"context"
	
	"testing"
)

// TestMCPTransport_Errors ensures type checks return errors and Is works.
func TestMCPTransport_Errors(t *testing.T) {
	tr := NewMCPTransport(nil)
	ctx := context.Background()
	// wrong provider for register
	if _, err := tr.RegisterToolProvider(ctx, &CliProvider{}); err == nil {
		t.Fatalf("expected error for wrong provider")
	}
	// wrong provider for deregister
	if err := tr.DeregisterToolProvider(ctx, &CliProvider{}); err == nil {
		t.Fatalf("expected error for wrong provider")
	}
	// wrong provider for call
	if _, err := tr.CallTool(ctx, "t", nil, &CliProvider{}, nil); err == nil {
		t.Fatalf("expected error for wrong provider")
	}
	// proper provider succeeds
	prov := NewMCPProvider("m", []string{"/home/raezil/go-utcp/examples/mcp_client/mcp_server"})
	if _, err := tr.RegisterToolProvider(ctx, prov); err != nil {
		t.Fatalf("register err: %v", err)
	}
	if res, err := tr.CallTool(ctx, "t", nil, prov, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else if res == nil {
		t.Fatalf("expected non-nil result")
	}
}



func TestMCPTransport_SuccessPaths(t *testing.T) {
	tr := NewMCPTransport(nil)
	ctx := context.Background()
	prov := NewMCPProvider("m", []string{"/home/raezil/go-utcp/examples/mcp_client/mcp_server"})
	if _, err := tr.RegisterToolProvider(ctx, prov); err != nil {
		t.Fatalf("register err: %v", err)
	}
	if err := tr.DeregisterToolProvider(ctx, prov); err != nil {
		t.Fatalf("deregister err: %v", err)
	}
}
