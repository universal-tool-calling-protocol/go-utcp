package utcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGraphQLClientTransport_SubscriptionFields ensures subscription fields are registered
func TestGraphQLClientTransport_SubscriptionFields(t *testing.T) {
	introspected := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query string `json:"query"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if strings.Contains(req.Query, "__schema") {
			introspected = true
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{"data": map[string]any{"__schema": map[string]any{
				"queryType":        map[string]any{"fields": []map[string]any{}},
				"mutationType":     map[string]any{"fields": []map[string]any{}},
				"subscriptionType": map[string]any{"fields": []map[string]any{{"name": "updates"}}},
			}}}
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"updates": []int{1}}})
	}))
	defer server.Close()

	prov := &GraphQLProvider{BaseProvider: BaseProvider{Name: "gql", ProviderType: ProviderGraphQL}, URL: server.URL}
	tr := NewGraphQLClientTransport(nil)
	ctx := context.Background()
	tools, err := tr.RegisterToolProvider(ctx, prov)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if !introspected {
		t.Fatalf("introspection not triggered")
	}
	found := false
	for _, tl := range tools {
		if tl.Name == "gql.updates" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("subscription field not registered: %+v", tools)
	}
}

// TestGraphQLClientTransport_OperationType verifies that the chosen operation type is respected
func TestGraphQLClientTransport_OperationType(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query string `json:"query"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		gotQuery = req.Query
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"ok": true}})
	}))
	defer server.Close()

	prov := &GraphQLProvider{URL: server.URL, OperationType: "subscription"}
	tr := NewGraphQLClientTransport(nil)
	if _, err := tr.CallTool(context.Background(), "ok", nil, prov, nil); err != nil {
		t.Fatalf("call error: %v", err)
	}
	if !strings.HasPrefix(gotQuery, "subscription ") {
		t.Fatalf("expected subscription operation, got %s", gotQuery)
	}

	prov.OperationType = "mutation"
	if _, err := tr.CallTool(context.Background(), "ok", nil, prov, nil); err != nil {
		t.Fatalf("call error: %v", err)
	}
	if !strings.HasPrefix(gotQuery, "mutation ") {
		t.Fatalf("expected mutation operation, got %s", gotQuery)
	}
}
