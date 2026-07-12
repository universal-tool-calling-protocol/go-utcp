package codemode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestDecideIfToolsNeeded(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		mockResponse   any
		mockError      error
		expectedNeeds  bool
		expectedError  bool
		responseIsJSON bool
	}{
		{
			name:           "LLM decides tools are needed",
			mockResponse:   `{"needs": true}`,
			expectedNeeds:  true,
			expectedError:  false,
			responseIsJSON: true,
		},
		{
			name:           "LLM decides tools are not needed",
			mockResponse:   `{"needs": false}`,
			expectedNeeds:  false,
			expectedError:  false,
			responseIsJSON: true,
		},
		{
			name:          "LLM returns an error",
			mockError:     errors.New("LLM error"),
			expectedNeeds: false,
			expectedError: true,
		},
		{
			name:           "LLM returns invalid JSON",
			mockResponse:   `{"needs": tru}`,
			expectedNeeds:  false,
			expectedError:  false,
			responseIsJSON: true,
		},
		{
			name:           "LLM returns non-JSON string",
			mockResponse:   "I don't know.",
			expectedNeeds:  false,
			expectedError:  false,
			responseIsJSON: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockModel{
				GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
					if tc.responseIsJSON {
						return tc.mockResponse, tc.mockError
					}
					return fmt.Sprintf("%v", tc.mockResponse), tc.mockError
				},
			}
			cm := CodeModeUTCP{model: mock}

			needs, err := cm.decideIfToolsNeeded(ctx, "some query", "some tools")

			if tc.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedNeeds, needs)
			}
		})
	}
}

func TestSelectTools(t *testing.T) {
	ctx := context.Background()
	mock := &mockModel{
		GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
			return `{"tools": ["tool1", "tool2"]}`, nil
		},
	}
	cm := &CodeModeUTCP{model: mock}

	selected, err := cm.selectTools(ctx, "some query", "some tools")

	require.NoError(t, err)
	assert.Equal(t, []string{"tool1", "tool2"}, selected)
}

func TestGenerateSnippet(t *testing.T) {
	ctx := context.Background()
	mockResp := struct {
		Code   string `json:"code"`
		Stream bool   `json:"stream"`
	}{
		Code:   `__out = "result"`,
		Stream: false,
	}
	respBytes, _ := json.Marshal(mockResp)

	mock := &mockModel{
		GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
			return string(respBytes), nil
		},
	}
	cm := &CodeModeUTCP{model: mock}

	snippet, stream, err := cm.generateSnippet(ctx, "query", []string{"tool1"}, "specs")

	require.NoError(t, err)
	assert.Equal(t, mockResp.Code, snippet)
	assert.Equal(t, mockResp.Stream, stream)
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

	assert.Contains(t, output, "TOOL: test.tool")
	assert.Contains(t, output, "DESCRIPTION: A test tool.")
	assert.Contains(t, output, "INPUT FIELDS (USE EXACTLY THESE KEYS):")
	assert.Contains(t, output, "- arg1: string")
	assert.Contains(t, output, "REQUIRED FIELDS:")
	assert.Contains(t, output, "FULL INPUT SCHEMA (JSON):")
	assert.Contains(t, output, "OUTPUT SCHEMA (EXACT SHAPE RETURNED BY TOOL):")
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
			assert.Equal(t, tc.expected, extractJSON(tc.input))
		})
	}
}

func TestIsValidSnippet(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{
			name:     "valid snippet",
			code:     `__out, err := codemode.CallTool("test", nil)`,
			expected: true,
		},
		{
			name:     "valid snippet with assignment",
			code:     `__out = "hello"`,
			expected: true,
		},
		{
			name:     "invalid due to map[value:]",
			code:     `__out = map[value:"hello"]`,
			expected: false,
		},
		{
			name:     "invalid due to missing __out",
			code:     `result, err := codemode.CallTool("test", nil)`,
			expected: false,
		},
		{
			name:     "empty code",
			code:     "",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isValidSnippet(tc.code))
		})
	}
}

func TestCallTool_NoToolsNeeded(t *testing.T) {
	ctx := context.Background()
	mock := &mockModel{
		GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
			return `{"tools": []}`, nil
		},
	}
	cm := &CodeModeUTCP{model: mock}

	needed, result, err := cm.CallTool(ctx, "a prompt that doesn't need tools")

	require.NoError(t, err)
	assert.False(t, needed)
	assert.Equal(t, "", result)
}

func TestCallTool_ToolsNeededAndExecuted(t *testing.T) {
	ctx := context.Background()
	modelCalls := 0

	mock := &mockModel{
		GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
			modelCalls++
			switch {
			case strings.Contains(prompt, "Select the UTCP tools required to fulfill the user's intent"):
				return `{"tools": ["codemode.run_code"]}`, nil
			case strings.Contains(prompt, "Generate a Go snippet"):
				return `{"code": "__out = \"success\""}`, nil
			default:
				return nil, fmt.Errorf("unexpected prompt: %s", prompt)
			}
		},
	}

	cm := &CodeModeUTCP{
		model: mock,
		executeFunc: func(ctx context.Context, args CodeModeArgs) (CodeModeResult, error) {
			require.Equal(t, `__out = "success"`, args.Code, "Code passed to Execute should match the generated snippet")
			return CodeModeResult{Value: "execution result"}, nil
		},
	}

	needed, result, err := cm.CallTool(ctx, "a prompt that needs tools")
	require.NoError(t, err)
	assert.True(t, needed, "Should indicate that tools were needed")
	assert.Equal(t, "execution result", result.(CodeModeResult).Value, "Should return the result from the mocked Execute function")
	assert.Equal(t, 2, modelCalls, "selection and code generation should be the only model calls")
}

func TestCallTool_GeneratesWithSelectedToolSpecsOnly(t *testing.T) {
	ctx := context.Background()
	client := &mockUTCP{
		searchToolsFn: func(query string, limit int) ([]tools.Tool, error) {
			return []tools.Tool{
				{Name: "selected.tool", Description: "Use this tool", Inputs: tools.ToolInputOutputSchema{Properties: map[string]any{"input": map[string]any{"type": "string"}}}},
				{Name: "unselected.tool", Description: "Do not include this tool", Inputs: tools.ToolInputOutputSchema{Properties: map[string]any{"secret": map[string]any{"type": "string"}}}},
			}, nil
		},
	}

	mock := &mockModel{
		GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
			switch {
			case strings.Contains(prompt, "Select the UTCP tools required to fulfill the user's intent"):
				assert.Contains(t, prompt, "selected.tool: Use this tool")
				assert.NotContains(t, prompt, "\"secret\"")
				return `{"tools": ["selected.tool"]}`, nil
			case strings.Contains(prompt, "Generate a Go snippet"):
				assert.Contains(t, prompt, "TOOL: selected.tool")
				assert.NotContains(t, prompt, "TOOL: unselected.tool")
				return `{"code": "__out = \"success\""}`, nil
			default:
				return nil, fmt.Errorf("unexpected prompt: %s", prompt)
			}
		},
	}

	cm := NewCodeModeUTCP(client, mock)
	cm.executeFunc = func(ctx context.Context, args CodeModeArgs) (CodeModeResult, error) {
		return CodeModeResult{Value: "execution result"}, nil
	}

	needed, _, err := cm.CallTool(ctx, "use the selected tool")
	require.NoError(t, err)
	assert.True(t, needed)
}

func TestCallTool_MultiStepExecution(t *testing.T) {
	ctx := context.Background()

	generatedCode := `
res1, err := codemode.CallTool("tool1", map[string]any{"param": "value1"})
if err != nil {
	__out = err.Error()
} else {
	res2, err := codemode.CallTool("tool2", map[string]any{"input": res1})
	if err != nil {
		__out = err.Error()
	} else {
		__out = res2
	}
}`

	mock := &mockModel{
		GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
			switch {
			case strings.Contains(prompt, "Select the UTCP tools required to fulfill the user's intent"):
				return `{"tools": ["codemode.run_code"]}`, nil
			case strings.Contains(prompt, "Generate a Go snippet"):
				return fmt.Sprintf(`{"code": %q}`, generatedCode), nil
			default:
				return nil, fmt.Errorf("unexpected prompt: %s", prompt)
			}
		},
	}

	cm := &CodeModeUTCP{
		model: mock,
		executeFunc: func(ctx context.Context, args CodeModeArgs) (CodeModeResult, error) {
			assert.Equal(t, strings.TrimSpace(generatedCode), args.Code)
			return CodeModeResult{Value: "tool2 result"}, nil
		},
	}

	needed, result, err := cm.CallTool(ctx, "a prompt that needs multiple tools and steps")
	require.NoError(t, err)
	assert.True(t, needed)
	assert.Equal(t, "tool2 result", result.(CodeModeResult).Value)
}

func TestCallTool_MixCallToolAndCallToolStream(t *testing.T) {
	ctx := context.Background()

	generatedCode := `
res1, err := codemode.CallTool("tool1", map[string]any{"param": "value1"})
if err != nil {
	__out = err.Error()
} else {
	res2Ch, err := codemode.CallToolStream("tool2", map[string]any{"input": res1})
	if err != nil {
		__out = err.Error()
	} else {
		var res2 []string
		for {
			item, ok := res2Ch.Next()
			if !ok {
				break
			}
			res2 += append(res2,item)
		}
		__out = res2
	}
}`

	mock := &mockModel{
		GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
			switch {
			case strings.Contains(prompt, "Select the UTCP tools required to fulfill the user's intent"):
				return `{"tools": ["codemode.run_code"]}`, nil
			case strings.Contains(prompt, "Generate a Go snippet"):
				return fmt.Sprintf(`{"code": %q}`, generatedCode), nil
			default:
				return nil, fmt.Errorf("unexpected prompt: %s", prompt)
			}
		},
	}

	cm := &CodeModeUTCP{
		model: mock,
		executeFunc: func(ctx context.Context, args CodeModeArgs) (CodeModeResult, error) {
			assert.Equal(t, strings.TrimSpace(generatedCode), args.Code)
			return CodeModeResult{Value: "tool2 stream result"}, nil
		},
	}
	needed, result, err := cm.CallTool(ctx, "a prompt that needs multiple tools and steps")
	require.NoError(t, err)
	assert.True(t, needed)
	assert.Equal(t, "tool2 stream result", result.(CodeModeResult).Value)
}

func TestCallTool_NoToolsSelected(t *testing.T) {
	ctx := context.Background()

	mock := &mockModel{
		GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
			switch {
			case strings.Contains(prompt, "Select the UTCP tools required to fulfill the user's intent"):
				return `{"tools": []}`, nil // No tools selected
			default:
				return nil, fmt.Errorf("unexpected prompt: %s", prompt)
			}
		},
	}

	cm := &CodeModeUTCP{model: mock}

	needed, result, err := cm.CallTool(ctx, "a prompt that doesn't need tools")

	require.NoError(t, err)
	assert.False(t, needed)
	assert.Equal(t, "", result)
}

func TestCallTool_GenerateSnippetFails(t *testing.T) {
	ctx := context.Background()

	mock := &mockModel{
		GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
			switch {
			case strings.Contains(prompt, "Select the UTCP tools required to fulfill the user's intent"):
				return `{"tools": ["codemode.run_code"]}`, nil
			case strings.Contains(prompt, "Generate a Go snippet"):
				return nil, errors.New("snippet generation failed") // Simulate snippet generation failure
			default:
				return nil, fmt.Errorf("unexpected prompt: %s", prompt)
			}
		},
	}

	cm := &CodeModeUTCP{model: mock}

	needed, _, err := cm.CallTool(ctx, "a prompt that needs tools")
	if err != nil {
		assert.EqualError(t, err, "snippet generation failed")
	}
	require.Error(t, err)
	assert.True(t, needed)
}
