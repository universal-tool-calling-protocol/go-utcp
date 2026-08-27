package codemode

import (
	"context"
	"strings"
	"testing"

	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

func TestPlanAndGenerate_UsesExprContract(t *testing.T) {
	var prompt string
	model := &mockModel{GenerateFunc: func(_ context.Context, p string) (any, error) {
		prompt = p
		return `{"tools":["tool1"],"code":"let r = codemode.CallTool(\"tool1\", {\"input\": \"hello\"}); r","stream":false}`, nil
	}}
	cm := &CodeModeUTCP{model: model}
	candidates := []tools.Tool{{
		Name: "tool1",
		Inputs: tools.ToolInputOutputSchema{Properties: map[string]any{
			"input": map[string]any{"type": "string"},
		}},
	}}

	plan, err := cm.planAndGenerate(context.Background(), "use tool1", candidates, renderUtcpToolsForPrompt(candidates))
	if err != nil {
		t.Fatalf("planAndGenerate returned error: %v", err)
	}
	if !strings.Contains(prompt, "EXPR CODEMODE") {
		t.Fatalf("expected Expr contract in planner prompt")
	}
	if strings.Contains(prompt, "CodeMode Go statements") || strings.Contains(prompt, "__out") || strings.Contains(prompt, " := ") {
		t.Fatalf("planner prompt still contains Go CodeMode instructions")
	}
	if plan.Code != `let r = codemode.CallTool("tool1", {"input": "hello"}); r` {
		t.Fatalf("unexpected Expr code: %s", plan.Code)
	}
}
