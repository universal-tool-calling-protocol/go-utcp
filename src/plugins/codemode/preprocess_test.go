package codemode

import (
	"testing"
)

func TestConvertOutWalrus(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple __out :=",
			input:    "__out := 5",
			expected: "__out = 5",
		},
		{
			name:     "__out := at start of line with spaces",
			input:    "  __out := \"hello\"",
			expected: "  __out = \"hello\"",
		},
		{
			name:     "multi-variable with __out first should NOT convert",
			input:    "__out, err := codemode.CallTool(\"test\", nil)",
			expected: "__out, err := codemode.CallTool(\"test\", nil)",
		},
		{
			name:     "nested in if block",
			input:    "if true {\n\t__out := 42\n}",
			expected: "if true {\n\t__out = 42\n}",
		},
		{
			name:     "other variable declarations should not be affected",
			input:    "result, err := codemode.CallTool(\"test\", nil)",
			expected: "result, err := codemode.CallTool(\"test\", nil)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := convertOutWalrus(tc.input)
			if result != tc.expected {
				t.Errorf("convertOutWalrus() = %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestPreprocessUserCode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple assignment",
			input:    "5 + 3",
			expected: "__out = 5 + 3",
		},
		{
			name:     "__out := should be converted",
			input:    "__out := 42",
			expected: "__out = 42",
		},
		{
			name:     "multi-line with __out, err :=",
			input:    "__out, err := codemode.CallTool(\"test\", nil)\nif err != nil { }",
			expected: "__out, err := codemode.CallTool(\"test\", nil)\nif err != nil { }",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := preprocessUserCode(tc.input)
			if result != tc.expected {
				t.Errorf("preprocessUserCode() = %q, want %q", result, tc.expected)
			}
		})
	}
}
