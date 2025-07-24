package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/http"
)

func TestHttpTransport_Register_InsecureURL(t *testing.T) {
	tr := NewHttpClientTransport(nil)
	prov := &HttpProvider{BaseProvider: BaseProvider{Name: "h", ProviderType: ProviderHTTP}, HTTPMethod: http.MethodGet, URL: "http://example.com"}
	if _, err := tr.RegisterToolProvider(context.Background(), prov); err == nil {
		t.Fatalf("expected security error")
	}
}

func TestHttpTransport_CallTool_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer server.Close()
	prov := &HttpProvider{BaseProvider: BaseProvider{Name: "h", ProviderType: ProviderHTTP}, HTTPMethod: http.MethodGet, URL: server.URL}
	tr := NewHttpClientTransport(nil)
	_, err := tr.CallTool(context.Background(), "t", map[string]any{}, prov, nil)
	if err == nil {
		t.Fatalf("expected error from call")
	}
}

func TestHttpTransport_CallTool_PathSub(t *testing.T) {
	gotPath := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	prov := &HttpProvider{BaseProvider: BaseProvider{Name: "h", ProviderType: ProviderHTTP}, HTTPMethod: http.MethodGet, URL: server.URL + "/{id}"}
	tr := NewHttpClientTransport(nil)
	res, err := tr.CallTool(context.Background(), "t", map[string]any{"id": 5}, prov, nil)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	if gotPath != "/5" {
		t.Fatalf("path substitution failed: %s", gotPath)
	}
	m := res.(map[string]interface{})
	if m["ok"] != true {
		t.Fatalf("unexpected result: %#v", res)
	}
}
