package tag

import (
	"context"
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
func TestDecodeToolsResponse(t *testing.T) {
	r := io.NopCloser(strings.NewReader(`{"tools":[{"name":"t"}]}`))
	tools, err := DecodeToolsResponse(r)
	if err != nil || len(tools) != 1 || tools[0].Name != "t" {
		t.Fatalf("decode err %v %v", tools, err)
	}
}
