package concepts

import (
	"context"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers"
)

func TestTagSearchStrategy_SearchTools(t *testing.T) {
	repo := &InMemoryToolRepository{
		tools: map[string][]Tool{
			"p": {
				{Name: "p.t1", Description: "first", Tags: []string{"alpha"}},
				{Name: "p.t2", Description: "second tool", Tags: []string{"beta"}},
			},
		},
		providers: map[string]Provider{},
	}
	strat := NewTagSearchStrategy(repo, 0.5)

	res, err := strat.SearchTools(context.Background(), "alpha", 2)
	if err != nil || len(res) == 0 || res[0].Name != "p.t1" {
		t.Fatalf("unexpected search result %v %v", res, err)
	}
}
