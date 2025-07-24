package tag

import (
	"context"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/repository"

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
