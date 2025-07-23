package tools

import (
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers"
)

// TestAddToolAndGetTools verifies AddTool and GetTools functions.
func TestAddToolAndGetTools(t *testing.T) {
	Tools = nil
	AddTool(Tool{Name: "t1"})
	AddTool(Tool{Name: "t2"})
	got := GetTools()
	if len(got) != 2 || got[0].Name != "t1" || got[1].Name != "t2" {
		t.Fatalf("unexpected tools slice: %+v", got)
	}
}

// TestAddToolPanics ensures AddTool panics when name is empty.
func TestAddToolPanics(t *testing.T) {
	defer func() { recover() }()
	AddTool(Tool{})
	t.Fatalf("expected panic for unnamed tool")
}

// TestRegisterToolDefaults uses RegisterTool with nil schemas and expects defaults.
func TestRegisterToolDefaults(t *testing.T) {
	Tools = nil
	handler := func(ctx map[string]interface{}, in map[string]interface{}) (map[string]interface{}, error) {
		return in, nil
	}
	prov := &CliProvider{BaseProvider: BaseProvider{Name: "cli", ProviderType: ProviderCLI}}
	RegisterTool(prov, "echo", "desc", []string{"tag"}, nil, nil, handler)
	if len(Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(Tools))
	}
	tool := Tools[0]
	if tool.Inputs.Type != "object" || tool.Outputs.Type != "object" {
		t.Fatalf("expected default schemas, got %+v %+v", tool.Inputs, tool.Outputs)
	}
}
