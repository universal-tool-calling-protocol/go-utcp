package utcp

import (
	"context"
	"os"
	"testing"
)

func TestTextClientTransport_RegisterAndCall(t *testing.T) {
	tmp, err := os.CreateTemp("", "manual*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.WriteString(`{"tools":[{"name":"hello","description":"Hi"}]}`)
	tmp.Close()

	prov := &TextProvider{
		BaseProvider: BaseProvider{Name: "text", ProviderType: ProviderText},
		FilePath:     tmp.Name(),
	}
	tr := NewTextTransport("")
	ctx := context.Background()
	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "text.hello" {
		t.Fatalf("unexpected tools: %+v", tools)
	}

	tr.tools["hello"] = Tool{Name: "hello", Handler: func(_ map[string]interface{}, _ map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"ok": true}, nil
	}}

	res, err := tr.CallTool(ctx, "text.hello", map[string]interface{}{"name": "K"}, prov, nil)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	m := res.(map[string]interface{})
	if m["ok"] != true {
		if m["greeting"] == nil {
			t.Fatalf("unexpected result: %#v", res)
		}
	}

	if err := tr.DeregisterToolProvider(ctx, prov); err != nil {
		t.Fatalf("deregister error: %v", err)
	}
	if len(tr.tools) != 0 {
		t.Fatalf("expected tools to be cleared")
	}
}
