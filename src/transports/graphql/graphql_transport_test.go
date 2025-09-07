package graphql

import (
	"context"
	json "github.com/universal-tool-calling-protocol/go-utcp/src/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/graphql"
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
				"mutationType": map[string]interface{}{"fields": []map[string]interface{}{}},
			}}}
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
	// Expect exactly one tool for the 'hello' query
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d: %+v", len(tools), tools)
	}

	res, err := tr.CallTool(ctx, "hello", map[string]interface{}{"name": "bob"}, prov, nil)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	if res != "world" {
		t.Fatalf("unexpected result: %#v", res)
	}
}

// TestGraphQLClientTransport_RegisterToolFiltering ensures that tools are
// filtered by provider OperationType and OperationName to avoid duplicates.
func TestGraphQLClientTransport_RegisterToolFiltering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query string `json:"query"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if strings.Contains(req.Query, "__schema") {
			// Return one query field and one subscription field
			resp := map[string]any{"data": map[string]any{"__schema": map[string]any{
				"queryType":        map[string]any{"fields": []map[string]any{{"name": "echo"}, {"name": "ping"}}},
				"subscriptionType": map[string]any{"fields": []map[string]any{{"name": "updates"}}},
			}}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	tr := NewGraphQLClientTransport(nil)
	ctx := context.Background()

	// Query provider should only register query fields and respect OperationName
	qName := "echo"
	provQuery := &GraphQLProvider{
		BaseProvider:  BaseProvider{Name: "gql", ProviderType: ProviderGraphQL},
		URL:           server.URL,
		OperationType: "query",
		OperationName: &qName,
	}
	tools, err := tr.RegisterToolProvider(ctx, provQuery)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "gql.echo" {
		t.Fatalf("unexpected tools: %#v", tools)
	}

	// Subscription provider should only register subscription field
	provSub := &GraphQLProvider{
		BaseProvider:  BaseProvider{Name: "gqlsub", ProviderType: ProviderGraphQL},
		URL:           server.URL,
		OperationType: "subscription",
	}
	tools, err = tr.RegisterToolProvider(ctx, provSub)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "gqlsub.updates" {
		t.Fatalf("unexpected tools for subscription: %#v", tools)
	}
}
