package cli

import (
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

func TestCliProvider_Basic(t *testing.T) {
	p := &CliProvider{
		BaseProvider: BaseProvider{Name: "cli", ProviderType: ProviderCLI},
		CommandName:  "echo",
	}
	if p.Type() != ProviderCLI {
		t.Fatalf("Type mismatch")
	}
	if p.Name != "cli" {
		t.Fatalf("Name mismatch")
	}
	if p.CommandName != "echo" {
		t.Fatalf("CommandName mismatch")
	}
}
