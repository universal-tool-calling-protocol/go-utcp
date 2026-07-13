package codemode

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

// mockModel simulates the behavior of an LLM for testing purposes.
type mockModel struct {
	GenerateFunc func(ctx context.Context, prompt string) (any, error)
}

func (m *mockModel) Generate(ctx context.Context, prompt string) (any, error) {
	if m.GenerateFunc != nil {
		return m.GenerateFunc(ctx, prompt)
	}
	return nil, errors.New("GenerateFunc not implemented")
}

func TestPlanAndGenerate_OneRoundTrip(t *testing.T) {
	ctx := context.Background()
	calls := 0
	code := `result, err := codemode.CallTool("tool1", map[string]any{"input": "hello"})
if err != nil {
	__out = err
	return __out
}
__out = result`

	mock := &mockModel{
		GenerateFunc: func(_ context.Context, prompt string) (any, error) {
			calls++
			if !strings.Contains(prompt, "Decide which UTCP tools are required and generate the complete CodeMode Go snippet") {
				t.Fatalf("combined planning prompt missing: %s", prompt)
			}
			if !strings.Contains(prompt, `"tool1"`) {
				t.Fatalf("candidate tool missing from prompt: %s", prompt)
			}
			if !strings.Contains(prompt, "TOOL: tool1") {
				t.Fatalf("tool schema missing from prompt: %s", prompt)
			}
			return fmt.Sprintf(`{"tools":["tool1"],"code":%q,"stream":false}`, code), nil
		},
	}
	cm := &CodeModeUTCP{model: mock}
	candidates := []tools.Tool{{
		Name:        "tool1",
		Description: "Test tool",
		Inputs: tools.ToolInputOutputSchema{
			Properties: map[string]any{"input": map[string]any{"type": "string"}},
		},
	}}

	plan, err := cm.planAndGenerate(ctx, "use tool1", candidates, renderUtcpToolsForPrompt(candidates))
	if err != nil {
		t.Fatalf("planAndGenerate returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one model call, got %d", calls)
	}
	if !reflect.DeepEqual(plan.Tools, []string{"tool1"}) {
		t.Fatalf("unexpected tools: %#v", plan.Tools)
	}
	if plan.Code != code {
		t.Fatalf("unexpected code:\n%s", plan.Code)
	}
	if plan.Stream {
		t.Fatal("expected non-streaming plan")
	}
}

func TestPlanAndGenerate_NoTools(t *testing.T) {
	mock := &mockModel{
		GenerateFunc: func(context.Context, string) (any, error) {
			return `{"tools":[],"code":"","stream":false}`, nil
		},
	}
	cm := &CodeModeUTCP{model: mock}
	candidates := []tools.Tool{{Name: "tool1"}}

	plan, err := cm.planAndGenerate(context.Background(), "answer directly", candidates, renderUtcpToolsForPrompt(candidates))
	if err != nil {
		t.Fatalf("planAndGenerate returned error: %v", err)
	}
	if len(plan.Tools) != 0 || plan.Code != "" || plan.Stream {
		t.Fatalf("expected empty plan, got %#v", plan)
	}
}

func TestPlanAndGenerate_Errors(t *testing.T) {
	candidates := []tools.Tool{{Name: "tool1"}}
	specs := renderUtcpToolsForPrompt(candidates)

	tests := []struct {
		name     string
		response any
		modelErr error
		wantErr  string
	}{
		{
			name:     "model error",
			modelErr: errors.New("provider unavailable"),
			wantErr:  "provider unavailable",
		},
		{
			name:     "no json",
			response: "not json",
			wantErr:  "plan generation returned no JSON",
		},
		{
			name:     "invalid json",
			response: `{"tools":[}`,
			wantErr:  "decode generated plan",
		},
		{
			name:     "selected tools with empty code",
			response: `{"tools":["tool1"],"code":"","stream":false}`,
			wantErr:  "generated plan selected tools but returned empty code",
		},
		{
			name:     "invalid snippet",
			response: `{"tools":["tool1"],"code":"result := 1","stream":false}`,
			wantErr:  "snippet validation failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockModel{
				GenerateFunc: func(context.Context, string) (any, error) {
					return tc.response, tc.modelErr
				},
			}
			cm := &CodeModeUTCP{model: mock}

			_, err := cm.planAndGenerate(context.Background(), "query", candidates, specs)
			if err == nil {
				t.Fatalf("expected error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err)
			}
		})
	}
}

func TestValidateGeneratedPlan(t *testing.T) {
	tests := []struct {
		name      string
		plan      generatedPlan
		used      []string
		allowed   []string
		wantError string
	}{
		{
			name:    "valid non-streaming plan",
			plan:    generatedPlan{Tools: []string{"tool1"}, Code: `__out, err := codemode.CallTool("tool1", nil)`},
			used:    []string{"tool1"},
			allowed: []string{"tool1", "tool2"},
		},
		{
			name:      "selected unavailable tool",
			plan:      generatedPlan{Tools: []string{"unknown"}, Code: `__out, err := codemode.CallTool("unknown", nil)`},
			used:      []string{"unknown"},
			allowed:   []string{"tool1"},
			wantError: `selected unavailable tool "unknown"`,
		},
		{
			name:      "code tool missing from declaration",
			plan:      generatedPlan{Tools: []string{}, Code: `__out, err := codemode.CallTool("tool1", nil)`},
			used:      []string{"tool1"},
			allowed:   []string{"tool1"},
			wantError: `references tool "tool1" missing from tools list`,
		},
		{
			name:      "declared unused tool",
			plan:      generatedPlan{Tools: []string{"tool1", "tool2"}, Code: `__out, err := codemode.CallTool("tool1", nil)`},
			used:      []string{"tool1"},
			allowed:   []string{"tool1", "tool2"},
			wantError: `declared unused tool "tool2"`,
		},
		{
			name: "stream flag mismatch",
			plan: generatedPlan{Tools: []string{"tool1"}, Code: `stream, err := codemode.CallToolStream("tool1", nil)
__out = stream`, Stream: false},
			used:      []string{"tool1"},
			allowed:   []string{"tool1"},
			wantError: "stream flag does not match",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGeneratedPlan(tc.plan, tc.used, tc.allowed)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
			}
		})
	}
}

func TestRankToolSpecs(t *testing.T) {
	specs := []tools.Tool{
		{Name: CodeModeToolName, Description: "must never be selected"},
		{Name: "memory.search", Description: "Search saved memories", Tags: []string{"memory", "search"}},
		{Name: "weather.current", Description: "Read current weather", Tags: []string{"weather"}},
		{Name: "memory.store", Description: "Store a memory", Tags: []string{"memory"}},
	}

	ranked := rankToolSpecs("search my memory", specs, 2)
	if len(ranked) != 2 {
		t.Fatalf("expected two candidates, got %d", len(ranked))
	}
	if ranked[0].Name != "memory.search" {
		t.Fatalf("expected memory.search first, got %s", ranked[0].Name)
	}
	for _, spec := range ranked {
		if spec.Name == CodeModeToolName {
			t.Fatal("codemode.run_code must not be a candidate")
		}
	}
}

func TestExtractGeneratedToolNames(t *testing.T) {
	code := `a, _ := codemode.CallTool("tool1", nil)
b, _ := codemode.CallToolStream("tool2", nil)
c, _ := codemode.CallTool("tool1", nil)
__out = []any{a, b, c}`

	got := extractGeneratedToolNames(code)
	want := []string{"tool1", "tool2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestRenderUtcpToolsForPrompt(t *testing.T) {
	specs := []tools.Tool{
		{
			Name:        "test.tool",
			Description: "A test tool.",
			Inputs: tools.ToolInputOutputSchema{
				Properties: map[string]any{
					"arg1": map[string]any{"type": "string"},
				},
				Required: []string{"arg1"},
			},
			Outputs: tools.ToolInputOutputSchema{
				Properties: map[string]any{
					"result": map[string]any{"type": "string"},
				},
			},
		},
	}

	output := renderUtcpToolsForPrompt(specs)
	checks := []string{
		"TOOL: test.tool",
		"DESCRIPTION: A test tool.",
		"INPUT FIELDS (USE EXACTLY THESE KEYS):",
		"- arg1: string",
		"REQUIRED FIELDS:",
		"FULL INPUT SCHEMA (JSON):",
		"OUTPUT SCHEMA (EXACT SHAPE RETURNED BY TOOL):",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Fatalf("expected rendered specs to contain %q", check)
		}
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"pure json", `{"key": "value"}`, `{"key": "value"}`},
		{"json with markdown", "```json\n{\"key\": \"value\"}\n```", `{"key": "value"}`},
		{"json with markdown no lang", "```\n{\"key\": \"value\"}\n```", `{"key": "value"}`},
		{"json with trailing text", `{"key": "value"} | some other text`, `{"key": "value"}`},
		{"nested json", `{"key": {"nested_key": "nested_value"}}`, `{"key": {"nested_key": "nested_value"}}`},
		{"text before json", `Here is the JSON: {"key": "value"}`, `{"key": "value"}`},
		{"empty string", "", ""},
		{"not a json", "just a string", ""},
		{"incomplete json", `{"key":`, ""},
		{"json with escaped quotes", `{"key": "value with \"quotes\""}`, `{"key": "value with \"quotes\""}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractJSON(tc.input); got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestIsValidSnippet(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{name: "valid snippet", code: `__out, err := codemode.CallTool("test", nil)`, expected: true},
		{name: "valid snippet with assignment", code: `__out = "hello"`, expected: true},
		{name: "invalid due to map[value:]", code: `__out = map[value:"hello"]`, expected: false},
		{name: "invalid due to missing __out", code: `result, err := codemode.CallTool("test", nil)`, expected: false},
		{name: "empty code", code: "", expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isValidSnippet(tc.code); got != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestCallTool_NoCandidateToolsSkipsModel(t *testing.T) {
	modelCalls := 0
	mock := &mockModel{
		GenerateFunc: func(context.Context, string) (any, error) {
			modelCalls++
			return nil, errors.New("model must not be called")
		},
	}
	cm := &CodeModeUTCP{model: mock}

	needed, result, err := cm.CallTool(context.Background(), "answer directly")
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if needed || result != "" {
		t.Fatalf("expected no tool result, got needed=%v result=%#v", needed, result)
	}
	if modelCalls != 0 {
		t.Fatalf("expected no model calls, got %d", modelCalls)
	}
}

func TestCallTool_NoToolsNeeded(t *testing.T) {
	client := &mockUTCP{
		searchToolsFn: func(string, int) ([]tools.Tool, error) {
			return []tools.Tool{{Name: "test.tool", Description: "A test tool"}}, nil
		},
	}
	modelCalls := 0
	mock := &mockModel{
		GenerateFunc: func(context.Context, string) (any, error) {
			modelCalls++
			return `{"tools":[],"code":"","stream":false}`, nil
		},
	}
	cm := NewCodeModeUTCP(client, mock)

	needed, result, err := cm.CallTool(context.Background(), "answer directly")
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if needed || result != "" {
		t.Fatalf("expected no tool result, got needed=%v result=%#v", needed, result)
	}
	if modelCalls != 1 {
		t.Fatalf("expected one combined model call, got %d", modelCalls)
	}
}

func TestCallTool_ToolsNeededAndExecutedInOneRoundTrip(t *testing.T) {
	client := &mockUTCP{
		searchToolsFn: func(string, int) ([]tools.Tool, error) {
			return []tools.Tool{{Name: "test.tool", Description: "Execute the test action"}}, nil
		},
	}
	modelCalls := 0
	generatedCode := `result, err := codemode.CallTool("test.tool", map[string]any{"input": "value"})
if err != nil {
	__out = err
	return __out
}
__out = result`
	mock := &mockModel{
		GenerateFunc: func(_ context.Context, prompt string) (any, error) {
			modelCalls++
			if !strings.Contains(prompt, "TOOL: test.tool") {
				t.Fatalf("expected selected tool schema in combined prompt")
			}
			return fmt.Sprintf(`{"tools":["test.tool"],"code":%q,"stream":false}`, generatedCode), nil
		},
	}
	cm := NewCodeModeUTCP(client, mock)
	cm.executeFunc = func(_ context.Context, args CodeModeArgs) (CodeModeResult, error) {
		if args.Code != generatedCode {
			t.Fatalf("unexpected generated code:\n%s", args.Code)
		}
		if args.Timeout != 20000 {
			t.Fatalf("expected 20000ms timeout, got %d", args.Timeout)
		}
		return CodeModeResult{Value: "execution result"}, nil
	}

	needed, result, err := cm.CallTool(context.Background(), "execute the test action")
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if !needed {
		t.Fatal("expected tools to be needed")
	}
	codeResult, ok := result.(CodeModeResult)
	if !ok || codeResult.Value != "execution result" {
		t.Fatalf("unexpected execution result: %#v", result)
	}
	if modelCalls != 1 {
		t.Fatalf("expected exactly one model call, got %d", modelCalls)
	}
}

func TestCallTool_UsesOnlyRankedCandidateSpecs(t *testing.T) {
	t.Setenv("UTCP_CODEMODE_CANDIDATE_LIMIT", "1")
	client := &mockUTCP{
		searchToolsFn: func(string, int) ([]tools.Tool, error) {
			return []tools.Tool{
				{
					Name:        "selected.tool",
					Description: "Use the selected input tool",
					Inputs: tools.ToolInputOutputSchema{
						Properties: map[string]any{"input": map[string]any{"type": "string"}},
					},
				},
				{
					Name:        "unselected.tool",
					Description: "Unrelated operation",
					Inputs: tools.ToolInputOutputSchema{
						Properties: map[string]any{"secret": map[string]any{"type": "string"}},
					},
				},
			}, nil
		},
	}
	generatedCode := `result, err := codemode.CallTool("selected.tool", map[string]any{"input": "value"})
if err != nil {
	__out = err
	return __out
}
__out = result`
	mock := &mockModel{
		GenerateFunc: func(_ context.Context, prompt string) (any, error) {
			if !strings.Contains(prompt, "TOOL: selected.tool") {
				t.Fatalf("selected tool missing from prompt")
			}
			if strings.Contains(prompt, "TOOL: unselected.tool") || strings.Contains(prompt, `"secret"`) {
				t.Fatalf("unselected schema leaked into prompt: %s", prompt)
			}
			return fmt.Sprintf(`{"tools":["selected.tool"],"code":%q,"stream":false}`, generatedCode), nil
		},
	}
	cm := NewCodeModeUTCP(client, mock)
	cm.executeFunc = func(context.Context, CodeModeArgs) (CodeModeResult, error) {
		return CodeModeResult{Value: "ok"}, nil
	}

	needed, _, err := cm.CallTool(context.Background(), "use the selected input tool")
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if !needed {
		t.Fatal("expected tools to be needed")
	}
}

func TestCallTool_MultiStepExecution(t *testing.T) {
	client := &mockUTCP{
		searchToolsFn: func(string, int) ([]tools.Tool, error) {
			return []tools.Tool{{Name: "tool1"}, {Name: "tool2"}}, nil
		},
	}
	generatedCode := `res1, err := codemode.CallTool("tool1", map[string]any{"param": "value1"})
if err != nil {
	__out = err
	return __out
}
res2, err := codemode.CallTool("tool2", map[string]any{"input": res1})
if err != nil {
	__out = err
	return __out
}
__out = res2`
	modelCalls := 0
	mock := &mockModel{
		GenerateFunc: func(context.Context, string) (any, error) {
			modelCalls++
			return fmt.Sprintf(`{"tools":["tool1","tool2"],"code":%q,"stream":false}`, generatedCode), nil
		},
	}
	cm := NewCodeModeUTCP(client, mock)
	cm.executeFunc = func(_ context.Context, args CodeModeArgs) (CodeModeResult, error) {
		if args.Code != generatedCode {
			t.Fatalf("unexpected generated code:\n%s", args.Code)
		}
		return CodeModeResult{Value: "tool2 result"}, nil
	}

	needed, result, err := cm.CallTool(context.Background(), "run tool1 then tool2")
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if !needed || result.(CodeModeResult).Value != "tool2 result" {
		t.Fatalf("unexpected result: needed=%v result=%#v", needed, result)
	}
	if modelCalls != 1 {
		t.Fatalf("expected one model call, got %d", modelCalls)
	}
}

func TestCallTool_MixCallToolAndCallToolStream(t *testing.T) {
	client := &mockUTCP{
		searchToolsFn: func(string, int) ([]tools.Tool, error) {
			return []tools.Tool{{Name: "tool1"}, {Name: "tool2"}}, nil
		},
	}
	generatedCode := `res1, err := codemode.CallTool("tool1", map[string]any{"param": "value1"})
if err != nil {
	__out = err
	return __out
}
stream, err := codemode.CallToolStream("tool2", map[string]any{"input": res1})
if err != nil {
	__out = err
	return __out
}
var items []any
for {
	item, err := stream.Next()
	if err != nil {
		break
	}
	items = append(items, item)
}
__out = items`
	mock := &mockModel{
		GenerateFunc: func(context.Context, string) (any, error) {
			return fmt.Sprintf(`{"tools":["tool1","tool2"],"code":%q,"stream":true}`, generatedCode), nil
		},
	}
	cm := NewCodeModeUTCP(client, mock)
	cm.executeFunc = func(_ context.Context, args CodeModeArgs) (CodeModeResult, error) {
		if args.Code != generatedCode {
			t.Fatalf("unexpected generated code:\n%s", args.Code)
		}
		return CodeModeResult{Value: "stream result"}, nil
	}

	needed, result, err := cm.CallTool(context.Background(), "run tool1 and stream tool2")
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if !needed || result.(CodeModeResult).Value != "stream result" {
		t.Fatalf("unexpected result: needed=%v result=%#v", needed, result)
	}
}

func TestCallTool_ModelFailure(t *testing.T) {
	client := &mockUTCP{
		searchToolsFn: func(string, int) ([]tools.Tool, error) {
			return []tools.Tool{{Name: "test.tool"}}, nil
		},
	}
	mock := &mockModel{
		GenerateFunc: func(context.Context, string) (any, error) {
			return nil, errors.New("plan generation failed")
		},
	}
	cm := NewCodeModeUTCP(client, mock)

	needed, _, err := cm.CallTool(context.Background(), "use test tool")
	if err == nil || err.Error() != "plan generation failed" {
		t.Fatalf("expected model error, got %v", err)
	}
	if needed {
		t.Fatal("model failure happens before a valid tool plan is established")
	}
}

func TestCallTool_RejectsToolMissingFromPlanList(t *testing.T) {
	client := &mockUTCP{
		searchToolsFn: func(string, int) ([]tools.Tool, error) {
			return []tools.Tool{{Name: "test.tool"}}, nil
		},
	}
	code := `result, err := codemode.CallTool("test.tool", nil)
__out = result`
	mock := &mockModel{
		GenerateFunc: func(context.Context, string) (any, error) {
			return fmt.Sprintf(`{"tools":[],"code":%q,"stream":false}`, code), nil
		},
	}
	cm := NewCodeModeUTCP(client, mock)

	needed, _, err := cm.CallTool(context.Background(), "use test tool")
	if err == nil || !strings.Contains(err.Error(), `missing from tools list`) {
		t.Fatalf("expected plan validation error, got %v", err)
	}
	if !needed {
		t.Fatal("generated code referenced a tool, so needed should be true")
	}
}
