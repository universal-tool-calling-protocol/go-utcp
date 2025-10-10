package adk

import (
	"context"
	"testing"
)

func TestAgentRegisterAndCall(t *testing.T) {
	agent := NewAgent("test-agent", WithDescription("demo"), WithVersion("2.0"))

	_, err := agent.RegisterTool(ToolDefinition{
		Name:        "echo",
		Description: "Echoes the provided message",
		Tags:        []string{"example"},
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			message, _ := input["message"].(string)
			if message == "" {
				message = ""
			}
			return map[string]any{"message": message}, nil
		},
	})
	if err != nil {
		t.Fatalf("failed to register tool: %v", err)
	}

	// Duplicate registration should fail.
	if _, err := agent.RegisterTool(ToolDefinition{Name: "echo", Handler: func(context.Context, map[string]any) (map[string]any, error) {
		return nil, nil
	}}); err == nil {
		t.Fatalf("expected duplicate registration to fail")
	}

	manual := agent.Manual()
	if manual.Version != "2.0" {
		t.Fatalf("unexpected manual version: %s", manual.Version)
	}
	if len(manual.Tools) != 1 {
		t.Fatalf("expected 1 tool in manual, got %d", len(manual.Tools))
	}
	if manual.Tools[0].Name != "echo" {
		t.Fatalf("unexpected tool name %s", manual.Tools[0].Name)
	}

	output, err := agent.Call(context.Background(), "echo", map[string]any{"message": "hi"})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if output["message"] != "hi" {
		t.Fatalf("unexpected output: %#v", output)
	}

	names := agent.ToolNames()
	if len(names) != 1 || names[0] != "echo" {
		t.Fatalf("unexpected tool names: %#v", names)
	}
}
