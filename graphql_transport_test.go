package utcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGraphQLClientTransport_RegisterAndCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if strings.Contains(req.Query, "__schema") {
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]interface{}{"data": map[string]interface{}{"__schema": map[string]interface{}{
				"queryType":    map[string]interface{}{"fields": []map[string]interface{}{{"name": "hello", "description": "hi"}}},
				"mutationType": map[string]interface{}{"fields": []map[string]interface{}{}}}}}
			json.NewEncoder(w).Encode(resp)
			return
		}
		if strings.Contains(req.Query, "hello") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{"hello": "world"}})
			return
		}
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	prov := &GraphQLProvider{
		BaseProvider: BaseProvider{Name: "gql", ProviderType: ProviderGraphQL},
		URL:          server.URL,
	}
	tr := NewGraphQLClientTransport(nil)
	ctx := context.Background()
	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("unexpected tools: %+v", tools)
	}

	res, err := tr.CallTool(ctx, "hello", map[string]interface{}{"name": "bob"}, prov, nil)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	if res != "world" {
		t.Fatalf("unexpected result: %#v", res)
	}
}
