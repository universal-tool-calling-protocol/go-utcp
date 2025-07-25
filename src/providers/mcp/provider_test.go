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
func TestMCPProvider_BuildersAndValidate(t *testing.T) {
	p := NewMCPProvider("name", []string{"cmd"})
	p.WithArgs("--x").WithEnv("A", "1").WithWorkingDir("/tmp").WithStdinData("in").WithTimeout(5).WithURL("http://x")
	if p.Timeout != 5 || p.Env["A"] != "1" || p.WorkingDir != "/tmp" || len(p.Args) != 1 || p.StdinData != "in" {
		t.Fatalf("builder mismatch: %+v", p)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("validate err: %v", err)
	}
	j := `{"name":"n","command":["cmd"]}`
	if _, err := NewMCPProviderFromJSON([]byte(j)); err != nil {
		t.Fatalf("from json err: %v", err)
	}
	bad := &MCPProvider{}
	if err := bad.Validate(); err == nil {
		t.Fatalf("expected validation error")
	}

	urlOnly := &MCPProvider{Name: "url", URL: "http://s"}
	if err := urlOnly.Validate(); err != nil {
		t.Fatalf("validate url err: %v", err)
	}
}
