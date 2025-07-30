package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/http"
)

func TestHttpClientTransport_RegisterAndCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manual":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"version":"1.0","tools":[{"name":"ping","description":"Ping"}]}`))
		case "/ping":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"pong":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	prov := &HttpProvider{
		BaseProvider: BaseProvider{Name: "test", ProviderType: ProviderHTTP},
		HTTPMethod:   http.MethodGet,
		URL:          server.URL + "/manual",
	}

	tr := NewHttpClientTransport(nil)
	ctx := context.Background()
	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "ping" {
		t.Fatalf("unexpected tools: %+v", tools)
	}

	prov.URL = server.URL + "/ping"
	res, err := tr.CallTool(ctx, "ping", map[string]any{}, prov)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	m, ok := res.(map[string]interface{})
	if !ok || m["pong"] != true {
		t.Fatalf("unexpected result: %#v", res)
	}
}
