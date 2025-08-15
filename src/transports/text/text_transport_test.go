package text

import (
	"context"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/text"
)

func TestTextTransport(t *testing.T) {
	tr := NewTextTransport(nil)
	prov := &providers.TextProvider{
		BaseProvider: BaseProvider{Name: "txt", ProviderType: ProviderText},
		Templates:    map[string]string{"hello": "Hello, {{.name}}"},
	}
	tools, err := tr.RegisterToolProvider(context.Background(), prov)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "hello" {
		t.Fatalf("unexpected tools: %v", tools)
	}
	res, err := tr.CallTool(context.Background(), "hello", map[string]any{"name": "Bob"}, prov, nil)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	if res.(string) != "Hello, Bob" {
		t.Fatalf("unexpected result %v", res)
	}
}
