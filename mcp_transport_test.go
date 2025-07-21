package utcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMCPClientTransport_RegisterAndCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := map[string]any{"version": "1.0", "tools": []any{map[string]any{"name": "foo"}}}
		_ = json.NewEncoder(w).Encode(m)
	}))
	defer server.Close()

	tr := NewMCPTransport(nil)
	prov := NewMCPProvider(server.URL)
	ctx := context.Background()
	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "foo" {
		t.Fatalf("unexpected tools: %v", tools)
	}

	if err := tr.DeregisterToolProvider(ctx, prov); err != nil {
		t.Fatalf("deregister error: %v", err)
	}

	if res, err := tr.CallTool(ctx, "foo", nil, prov, nil); err != nil {
		t.Fatalf("call error: %v", err)
	} else if res == nil {
		t.Fatalf("expected non-nil result")
	}
}
