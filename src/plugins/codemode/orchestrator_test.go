package codemode

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

type mockModel struct {
	GenerateFunc func(context.Context, string) (any, error)
}

func (m *mockModel) Generate(ctx context.Context, prompt string) (any, error) {
	if m.GenerateFunc == nil { return nil, errors.New("GenerateFunc not implemented") }
	return m.GenerateFunc(ctx, prompt)
}

func TestPlanAndGenerate_ExprRoundTrip(t *testing.T) {
	code := `let result = codemode.CallTool("tool1", {"input": "hello"}); result`
	var prompt string
	mock := &mockModel{GenerateFunc: func(_ context.Context, p string) (any, error) {
		prompt = p
		return `{"tools":["tool1"],"code":"let result = codemode.CallTool(\"tool1\", {\"input\": \"hello\"}); result","stream":false}`, nil
	}}
	cm := &CodeModeUTCP{model: mock}
	candidates := []tools.Tool{{Name: "tool1", Description: "Test tool", Inputs: tools.ToolInputOutputSchema{Properties: map[string]any{"input": map[string]any{"type": "string"}}}}}
	plan, err := cm.planAndGenerate(context.Background(), "use tool1", candidates, renderUtcpToolsForPrompt(candidates))
	if err != nil { t.Fatalf("planAndGenerate returned error: %v", err) }
	if !strings.Contains(prompt, "EXPR CODEMODE CONTRACT") { t.Fatal("missing Expr contract") }
	if strings.Contains(prompt, "CodeMode Go") || strings.Contains(prompt, "__out") || strings.Contains(prompt, " := ") { t.Fatal("prompt contains Go-oriented CodeMode instructions") }
	if plan.Code != code { t.Fatalf("unexpected code: %s", plan.Code) }
}

func TestPlanAndGenerate_NoTools(t *testing.T) {
	cm := &CodeModeUTCP{model: &mockModel{GenerateFunc: func(context.Context, string) (any, error) {
		return `{"tools":[],"code":"","stream":false}`, nil
	}}}
	plan, err := cm.planAndGenerate(context.Background(), "answer directly", []tools.Tool{{Name: "tool1"}}, "TOOL: tool1")
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if len(plan.Tools) != 0 || plan.Code != "" || plan.Stream { t.Fatalf("unexpected plan: %#v", plan) }
}

func TestPlanAndGenerate_Errors(t *testing.T) {
	cases := []struct{name string; response any; modelErr error; want string}{
		{"model error", nil, errors.New("provider unavailable"), "provider unavailable"},
		{"no json", "not json", nil, "plan generation returned no JSON"},
		{"invalid json", `{"tools":[}`, nil, "decode generated plan"},
		{"tools without code", `{"tools":["tool1"],"code":"","stream":false}`, nil, "generated plan selected tools but returned empty code"},
		{"invalid Expr", `{"tools":["tool1"],"code":"result := 1","stream":false}`, nil, "snippet validation failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm := &CodeModeUTCP{model: &mockModel{GenerateFunc: func(context.Context, string) (any, error) { return tc.response, tc.modelErr }}}
			_, err := cm.planAndGenerate(context.Background(), "query", []tools.Tool{{Name: "tool1"}}, "TOOL: tool1")
			if err == nil || !strings.Contains(err.Error(), tc.want) { t.Fatalf("expected %q, got %v", tc.want, err) }
		})
	}
}

func TestValidateGeneratedPlan_Expr(t *testing.T) {
	valid := generatedPlan{Tools: []string{"tool1"}, Code: `let r = codemode.CallTool("tool1", {}); r`}
	if err := validateGeneratedPlan(valid, []string{"tool1"}, []string{"tool1", "tool2"}); err != nil { t.Fatalf("valid plan rejected: %v", err) }
	bad := generatedPlan{Tools: []string{"unknown"}, Code: `codemode.CallTool("unknown", {})`}
	if err := validateGeneratedPlan(bad, []string{"unknown"}, []string{"tool1"}); err == nil { t.Fatal("expected unavailable-tool error") }
	missing := generatedPlan{Tools: nil, Code: `codemode.CallTool("tool1", {})`}
	if err := validateGeneratedPlan(missing, []string{"tool1"}, []string{"tool1"}); err == nil || !strings.Contains(err.Error(), "missing from tools list") { t.Fatalf("unexpected error: %v", err) }
	unused := generatedPlan{Tools: []string{"tool1", "tool2"}, Code: `codemode.CallTool("tool1", {})`}
	if err := validateGeneratedPlan(unused, []string{"tool1"}, []string{"tool1", "tool2"}); err == nil || !strings.Contains(err.Error(), "unused tool") { t.Fatalf("unexpected error: %v", err) }
}

func TestValidateGeneratedPlan_StreamFlag(t *testing.T) {
	plan := generatedPlan{Tools: []string{"tool1"}, Code: `codemode.CallToolStream("tool1", {})`, Stream: false}
	if err := validateGeneratedPlan(plan, []string{"tool1"}, []string{"tool1"}); err == nil || !strings.Contains(err.Error(), "stream flag") { t.Fatalf("unexpected error: %v", err) }
}

func TestRankToolSpecs(t *testing.T) {
	specs := []tools.Tool{{Name: CodeModeToolName}, {Name: "memory.search", Description: "Search memories", Tags: []string{"memory", "search"}}, {Name: "weather.current", Description: "Current weather"}}
	ranked := rankToolSpecs("search memory", specs, 1)
	if len(ranked) != 1 || ranked[0].Name != "memory.search" { t.Fatalf("unexpected ranking: %#v", ranked) }
}

func TestExtractGeneratedToolNames(t *testing.T) {
	code := `let a = codemode.CallTool("tool1", {}); let b = codemode.CallToolStream("tool2", {}); [a, b]`
	got := extractGeneratedToolNames(code)
	if !reflect.DeepEqual(got, []string{"tool1", "tool2"}) { t.Fatalf("unexpected names: %#v", got) }
}

func TestRenderUtcpToolsForPrompt(t *testing.T) {
	spec := tools.Tool{Name: "test.tool", Description: "A test tool", Inputs: tools.ToolInputOutputSchema{Properties: map[string]any{"arg1": map[string]any{"type": "string"}}, Required: []string{"arg1"}}}
	output := renderUtcpToolsForPrompt([]tools.Tool{spec})
	for _, expected := range []string{"TOOL: test.tool", "DESCRIPTION: A test tool", "- arg1: string", "REQUIRED FIELDS:", "FULL INPUT SCHEMA (JSON):"} { if !strings.Contains(output, expected) { t.Fatalf("missing %q", expected) } }
}

func TestIsValidSnippet_Expr(t *testing.T) {
	valid := []string{`42`, `let value = 42; value`, `codemode.CallTool("tool1", {})`, `let r = codemode.CallTool("tool1", {}); r`}
	for _, code := range valid { if !isValidSnippet(code) { t.Fatalf("expected valid Expr: %s", code) } }
	invalid := []string{`package main`, `import "fmt"`, `result, err := codemode.CallTool("tool1", nil)`, `CallTool("tool1", {})`, `codemode.SearchTools("tool1")`, `codemode.CallTool("tool1", {})
__out`}
	for _, code := range invalid { if isValidSnippet(code) { t.Fatalf("expected invalid Expr: %s", code) } }
}

func TestCallTool_NoCandidateToolsSkipsModel(t *testing.T) {
	calls := 0
	cm := &CodeModeUTCP{model: &mockModel{GenerateFunc: func(context.Context, string) (any, error) { calls++; return nil, errors.New("must not call") }}}
	needed, result, err := cm.CallTool(context.Background(), "answer directly")
	if err != nil || needed || result != "" || calls != 0 { t.Fatalf("unexpected result: needed=%v result=%#v err=%v calls=%d", needed, result, err, calls) }
}

func TestCallTool_ExprExecution(t *testing.T) {
	client := &mockUTCP{searchToolsFn: func(string, int) ([]tools.Tool, error) { return []tools.Tool{{Name: "test.tool", Description: "execute test"}}, nil }}
	calls := 0
	mock := &mockModel{GenerateFunc: func(_ context.Context, prompt string) (any, error) {
		calls++
		if !strings.Contains(prompt, "TOOL: test.tool") { return nil, errors.New("tool schema missing") }
		return `{"tools":["test.tool"],"code":"codemode.CallTool(\"test.tool\", {})","stream":false}`, nil
	}}
	cm := NewCodeModeUTCP(client, mock)
	cm.executeFunc = func(_ context.Context, args CodeModeArgs) (CodeModeResult, error) {
		if args.Code != `codemode.CallTool("test.tool", {})` { t.Fatalf("unexpected Expr: %s", args.Code) }
		return CodeModeResult{Value: "ok"}, nil
	}
	needed, result, err := cm.CallTool(context.Background(), "execute test")
	if err != nil || !needed || calls != 1 { t.Fatalf("unexpected result: needed=%v result=%#v err=%v calls=%d", needed, result, err, calls) }
	if result.(CodeModeResult).Value != "ok" { t.Fatalf("unexpected result: %#v", result) }
}
