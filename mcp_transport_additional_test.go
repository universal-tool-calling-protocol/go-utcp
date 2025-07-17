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
	// proper provider returns ErrNotImplemented
	_, err := tr.CallTool(ctx, "t", nil, NewMCPProvider("m"), nil)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
}

// TestErrNotImplemented verifies Error and Is behaviour.
func TestErrNotImplemented(t *testing.T) {
	var e error = ErrNotImplemented
	if !errors.Is(e, ErrNotImplemented) {
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
