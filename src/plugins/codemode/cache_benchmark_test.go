package codemode

import (
	"context"
	"testing"

	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

// Benchmark tool specs without cache
func BenchmarkToolSpecs_NoCache(b *testing.B) {
	mock := &mockUTCP{
		searchToolsFn: func(query string, limit int) ([]tools.Tool, error) {
			// Simulate SearchTools returning a large list
			result := make([]tools.Tool, 50)
			for i := 0; i < 50; i++ {
				result[i] = tools.Tool{
					Name:        "test.tool" + string(rune(i)),
					Description: "Test tool " + string(rune(i)),
				}
			}
			return result, nil
		},
	}

	mockModel := &mockModel{
		GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
			return "{}", nil
		},
	}

	cm := NewCodeModeUTCP(mock, mockModel)
	// Disable cache to benchmark without caching
	cm.cache = nil

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cm.ToolSpecs()
	}
}

// Benchmark tool specs with cache (first call - miss)
func BenchmarkToolSpecs_WithCache_Miss(b *testing.B) {
	mock := &mockUTCP{
		searchToolsFn: func(query string, limit int) ([]tools.Tool, error) {
			result := make([]tools.Tool, 50)
			for i := 0; i < 50; i++ {
				result[i] = tools.Tool{
					Name:        "test.tool" + string(rune(i)),
					Description: "Test tool " + string(rune(i)),
				}
			}
			return result, nil
		},
	}

	mockModel := &mockModel{
		GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
			return "{}", nil
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		cm := NewCodeModeUTCP(mock, mockModel)
		b.StartTimer()
		_ = cm.ToolSpecs()
	}
}

// Benchmark tool specs with cache (subsequent calls - hits)
func BenchmarkToolSpecs_WithCache_Hit(b *testing.B) {
	mock := &mockUTCP{
		searchToolsFn: func(query string, limit int) ([]tools.Tool, error) {
			result := make([]tools.Tool, 50)
			for i := 0; i < 50; i++ {
				result[i] = tools.Tool{
					Name:        "test.tool" + string(rune(i)),
					Description: "Test tool " + string(rune(i)),
				}
			}
			return result, nil
		},
	}

	mockModel := &mockModel{
		GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
			return "{}", nil
		},
	}

	cm := NewCodeModeUTCP(mock, mockModel)
	// Warm up cache
	_ = cm.ToolSpecs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cm.ToolSpecs()
	}
}

// Benchmark selectTools without cache
func BenchmarkSelectTools_NoCache(b *testing.B) {
	mock := &mockUTCP{}
	mockModel := &mockModel{
		GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
			return `{"tools": ["test.tool1", "test.tool2"]}`, nil
		},
	}

	cm := NewCodeModeUTCP(mock, mockModel)
	// Disable cache
	cm.cache = nil

	ctx := context.Background()
	query := "find memory tools"
	toolsStr := "test.tool1, test.tool2, test.tool3"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cm.selectTools(ctx, query, toolsStr)
	}
}

// Benchmark selectTools with cache (first call - miss)
func BenchmarkSelectTools_WithCache_Miss(b *testing.B) {
	mock := &mockUTCP{}
	mockModel := &mockModel{
		GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
			return `{"tools": ["test.tool1", "test.tool2"]}`, nil
		},
	}

	ctx := context.Background()
	query := "find memory tools"
	toolsStr := "test.tool1, test.tool2, test.tool3"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		cm := NewCodeModeUTCP(mock, mockModel)
		b.StartTimer()
		_, _ = cm.selectTools(ctx, query, toolsStr)
	}
}

// Benchmark selectTools with cache (subsequent calls - hits)
func BenchmarkSelectTools_WithCache_Hit(b *testing.B) {
	mock := &mockUTCP{}
	mockModel := &mockModel{
		GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
			return `{"tools": ["test.tool1", "test.tool2"]}`, nil
		},
	}

	cm := NewCodeModeUTCP(mock, mockModel)

	ctx := context.Background()
	query := "find memory tools"
	toolsStr := "test.tool1, test.tool2, test.tool3"

	// Warm up cache
	_, _ = cm.selectTools(ctx, query, toolsStr)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cm.selectTools(ctx, query, toolsStr)
	}
}

// Benchmark full CallTool workflow with cache
func BenchmarkCallTool_WithCache(b *testing.B) {
	mock := &mockUTCP{
		searchToolsFn: func(query string, limit int) ([]tools.Tool, error) {
			return []tools.Tool{
				{Name: "test.tool1", Description: "Test tool 1"},
				{Name: "test.tool2", Description: "Test tool 2"},
			}, nil
		},
	}

	mockModel := &mockModel{
		GenerateFunc: func(ctx context.Context, prompt string) (any, error) {
			// Simulate different responses for different stages
			if stringContains(prompt, "Decide if") {
				return `{"needs": true}`, nil
			}
			if stringContains(prompt, "Select ALL UTCP tools") {
				return `{"tools": ["test.tool1"]}`, nil
			}
			if stringContains(prompt, "Generate a Go snippet") {
				return `{"code": "__out = map[string]any{\"result\": \"test\"}", "stream": false}`, nil
			}
			return "{}", nil
		},
	}

	cm := NewCodeModeUTCP(mock, mockModel)
	// Mock execute to avoid actual code execution
	cm.executeFunc = func(ctx context.Context, args CodeModeArgs) (CodeModeResult, error) {
		return CodeModeResult{Value: "mocked result"}, nil
	}

	ctx := context.Background()
	query := "test query"

	// First call to warm up cache
	_, _, _ = cm.CallTool(ctx, query)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = cm.CallTool(ctx, query)
	}
}

// Helper function
func stringContains(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
