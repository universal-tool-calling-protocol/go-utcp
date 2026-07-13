package codemode

import (
	"context"
	"fmt"
	"testing"

	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

func benchmarkTools(count int) []tools.Tool {
	result := make([]tools.Tool, count)
	for i := 0; i < count; i++ {
		result[i] = tools.Tool{
			Name:        fmt.Sprintf("test.tool%d", i),
			Description: fmt.Sprintf("Test tool %d for memory search and processing", i),
			Tags:        []string{"test", "memory"},
			Inputs: tools.ToolInputOutputSchema{
				Properties: map[string]any{
					"query": map[string]any{"type": "string"},
				},
			},
		}
	}
	return result
}

// Benchmark tool specs without cache.
func BenchmarkToolSpecs_NoCache(b *testing.B) {
	mock := &mockUTCP{
		searchToolsFn: func(string, int) ([]tools.Tool, error) {
			return benchmarkTools(50), nil
		},
	}
	cm := NewCodeModeUTCP(mock, &mockModel{})
	cm.cache = nil

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cm.ToolSpecs()
	}
}

// Benchmark tool specs with a fresh cache on every iteration.
func BenchmarkToolSpecs_WithCache_Miss(b *testing.B) {
	mock := &mockUTCP{
		searchToolsFn: func(string, int) ([]tools.Tool, error) {
			return benchmarkTools(50), nil
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		cm := NewCodeModeUTCP(mock, &mockModel{})
		b.StartTimer()
		_ = cm.ToolSpecs()
	}
}

// Benchmark tool specs after the cache has been populated.
func BenchmarkToolSpecs_WithCache_Hit(b *testing.B) {
	mock := &mockUTCP{
		searchToolsFn: func(string, int) ([]tools.Tool, error) {
			return benchmarkTools(50), nil
		},
	}
	cm := NewCodeModeUTCP(mock, &mockModel{})
	_ = cm.ToolSpecs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cm.ToolSpecs()
	}
}

// Benchmark the deterministic local candidate-ranking stage.
func BenchmarkRankToolSpecs(b *testing.B) {
	specs := benchmarkTools(100)
	query := "search memory and process the result"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rankToolSpecs(query, specs, 16)
	}
}

// Benchmark one combined selection-and-generation model call with a mock model.
func BenchmarkPlanAndGenerate_OneRoundTrip(b *testing.B) {
	candidates := benchmarkTools(16)
	toolSpecs := renderUtcpToolsForPrompt(candidates)
	response := `{"tools":["test.tool1"],"code":"result, err := codemode.CallTool(\"test.tool1\", map[string]any{\"query\": \"memory\"})\nif err != nil {\n__out = err\nreturn __out\n}\n__out = result","stream":false}`
	mockModel := &mockModel{
		GenerateFunc: func(context.Context, string) (any, error) {
			return response, nil
		},
	}
	cm := NewCodeModeUTCP(&mockUTCP{}, mockModel)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cm.planAndGenerate(ctx, "search memory", candidates, toolSpecs)
	}
}

// Benchmark the full orchestration path. Tool specs are cached, while each
// request intentionally performs exactly one model roundtrip.
func BenchmarkCallTool_OneRoundTrip(b *testing.B) {
	mock := &mockUTCP{
		searchToolsFn: func(string, int) ([]tools.Tool, error) {
			return []tools.Tool{
				{
					Name:        "test.tool1",
					Description: "Search memory",
					Inputs: tools.ToolInputOutputSchema{
						Properties: map[string]any{"query": map[string]any{"type": "string"}},
					},
				},
				{Name: "test.tool2", Description: "Another tool"},
			}, nil
		},
	}
	response := `{"tools":["test.tool1"],"code":"result, err := codemode.CallTool(\"test.tool1\", map[string]any{\"query\": \"memory\"})\nif err != nil {\n__out = err\nreturn __out\n}\n__out = result","stream":false}`
	mockModel := &mockModel{
		GenerateFunc: func(context.Context, string) (any, error) {
			return response, nil
		},
	}
	cm := NewCodeModeUTCP(mock, mockModel)
	cm.executeFunc = func(context.Context, CodeModeArgs) (CodeModeResult, error) {
		return CodeModeResult{Value: "mocked result"}, nil
	}
	ctx := context.Background()
	query := "search memory"

	// Warm only the tool-spec/catalog cache. The generated plan is deliberately
	// not cached, so one model call remains part of every benchmark iteration.
	_, _ = cm.toolSpecsAndCatalog()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = cm.CallTool(ctx, query)
	}
}
