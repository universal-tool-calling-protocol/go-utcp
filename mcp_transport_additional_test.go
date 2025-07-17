package UTCP

import (
	"context"
	"errors"
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
	// proper provider returns notImplErr
	_, err := tr.CallTool(ctx, "t", nil, NewMCPProvider("m"), nil)
	if !errors.Is(err, notImplErr{}) {
		t.Fatalf("expected notImplErr, got %v", err)
	}
}

// TestNotImplErr verifies Error and Is behaviour.
func TestNotImplErr(t *testing.T) {
	var e error = notImplErr{}
	target := errors.New("MCP transport invocation not implemented yet")
	if !errors.Is(e, target) {
		t.Fatalf("errors.Is failed")
	}
	if e.Error() == "" {
		t.Fatalf("empty error message")
	}
}

func TestMCPTransport_SuccessPaths(t *testing.T) {
	tr := NewMCPTransport(nil)
	ctx := context.Background()
	prov := NewMCPProvider("m")
	if _, err := tr.RegisterToolProvider(ctx, prov); err != nil {
		t.Fatalf("register err: %v", err)
	}
	if err := tr.DeregisterToolProvider(ctx, prov); err != nil {
		t.Fatalf("deregister err: %v", err)
	}
}
