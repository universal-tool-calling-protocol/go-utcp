package tag

import (
	"context"
	"fmt"
	"io"
	"strings"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/repository"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"

	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

func TestTagSearchStrategy_SearchTools(t *testing.T) {
	repo := &InMemoryToolRepository{
		Tools: map[string][]Tool{
			"p": {
				{Name: "p.t1", Description: "first", Tags: []string{"alpha"}},
				{Name: "p.t2", Description: "second tool", Tags: []string{"beta"}},
			},
		},
		Providers: map[string]Provider{},
	}
	strat := NewTagSearchStrategy(repo, 0.5)

	res, err := strat.SearchTools(context.Background(), "alpha", 2)
	if err != nil || len(res) == 0 || res[0].Name != "p.t1" {
		t.Fatalf("unexpected search result %v %v", res, err)
	}
}

func TestTagSearchStrategy_UnlimitedAndCancelled(t *testing.T) {
	repo := &InMemoryToolRepository{
		Tools: map[string][]Tool{
			"p": {
				{Name: "p.t1", Tags: []string{"alpha"}},
				{Name: "p.t2", Tags: []string{"alpha"}},
			},
		},
		Providers: map[string]Provider{},
	}
	strategy := NewTagSearchStrategy(repo, 0.5)
	results, err := strategy.SearchTools(context.Background(), "alpha", 0)
	if err != nil || len(results) != 2 {
		t.Fatalf("unlimited search returned %d results: %v", len(results), err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := strategy.SearchTools(ctx, "alpha", 1); err == nil {
		t.Fatal("expected cancelled search to fail")
	}
}
func TestDecodeToolsResponse(t *testing.T) {
	r := io.NopCloser(strings.NewReader(`{"tools":[{"name":"t"}]}`))
	tools, err := DecodeToolsResponse(r)
	if err != nil || len(tools) != 1 || tools[0].Name != "t" {
		t.Fatalf("decode err %v %v", tools, err)
	}
}

func BenchmarkTagSearchStrategySearchTools(b *testing.B) {
	repo := NewInMemoryToolRepository()
	ctx := context.Background()
	for providerIndex := 0; providerIndex < 10; providerIndex++ {
		name := fmt.Sprintf("provider_%02d", providerIndex)
		provider := &benchmarkProvider{BaseProvider{Name: name, ProviderType: ProviderType("benchmark")}}
		providerTools := make([]Tool, 100)
		for toolIndex := range providerTools {
			providerTools[toolIndex] = Tool{
				Name:        fmt.Sprintf("%s.tool_%03d", name, toolIndex),
				Description: "Search memory records and process matching documents",
				Tags:        []string{"memory", "search", "documents"},
			}
		}
		repo.(*InMemoryToolRepository).Providers[name] = provider
		repo.(*InMemoryToolRepository).Tools[name] = providerTools
	}
	strategy := NewTagSearchStrategy(repo, 0.5)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matches, err := strategy.SearchTools(ctx, "search memory documents", 16)
		if err != nil || len(matches) != 16 {
			b.Fatalf("search failed: matches=%d err=%v", len(matches), err)
		}
	}
}

type benchmarkProvider struct{ BaseProvider }
