package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	"github.com/universal-tool-calling-protocol/go-utcp/src/providers/cli"
	"github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

func benchmarkRepository(b *testing.B, providerCount, toolsPerProvider int) *InMemoryToolRepository {
	b.Helper()
	repo := NewInMemoryToolRepository().(*InMemoryToolRepository)
	ctx := context.Background()
	for providerIndex := 0; providerIndex < providerCount; providerIndex++ {
		name := fmt.Sprintf("provider_%02d", providerIndex)
		provider := &cli.CliProvider{BaseProvider: base.BaseProvider{Name: name, ProviderType: base.ProviderCLI}}
		providerTools := make([]tools.Tool, toolsPerProvider)
		for toolIndex := range providerTools {
			providerTools[toolIndex].Name = fmt.Sprintf("%s.tool_%04d", name, toolIndex)
		}
		if err := repo.SaveProviderWithTools(ctx, provider, providerTools); err != nil {
			b.Fatal(err)
		}
	}
	return repo
}

func BenchmarkInMemoryToolRepositoryGetTool(b *testing.B) {
	repo := benchmarkRepository(b, 10, 100)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tool, err := repo.GetTool(ctx, "provider_09.tool_0099")
		if err != nil || tool == nil {
			b.Fatalf("lookup failed: tool=%v err=%v", tool, err)
		}
	}
}

func BenchmarkInMemoryToolRepositoryGetTools(b *testing.B) {
	repo := benchmarkRepository(b, 10, 100)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		all, err := repo.GetTools(ctx)
		if err != nil || len(all) != 1_000 {
			b.Fatalf("list failed: count=%d err=%v", len(all), err)
		}
	}
}
