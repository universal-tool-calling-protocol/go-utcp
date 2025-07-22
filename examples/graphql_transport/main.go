// examples/graphql_transport/main.go
// A simple example demonstrating how to use GraphQLClientTransport
// This version also spins up a local mock GraphQL server at http://localhost:8080/graphql

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	providers "github.com/universal-tool-calling-protocol/go-utcp/concepts/providers"
	utcp "github.com/universal-tool-calling-protocol/go-utcp/concepts/transports/graphql"
)

// --- Mock server implementation ---
func startMockServer(addr string) {
	http.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "only POST allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "could not read body", http.StatusBadRequest)
			return
		}
		var req struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		// Handle introspection queries
		if strings.Contains(req.Query, "__schema") {
			// Return minimal schema with launchesPast field
			schema := map[string]interface{}{
				"__schema": map[string]interface{}{
					"queryType": map[string]interface{}{ // root queries
						"name": "Query",
						"fields": []map[string]interface{}{ // list of available fields
							{"name": "launchesPast", "description": "Mocked launchesPast field"},
						},
					},
					"mutationType": nil,
				},
			}
			resp := map[string]interface{}{"data": schema}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Very naive routing based on presence of "launchesPast" in the query
		if strings.Contains(req.Query, "launchesPast") {
			// Prepare a fake list of 3 launches
			launches := []map[string]interface{}{
				{"id": "1", "mission_name": "FalconSat"},
				{"id": "2", "mission_name": "DemoSat"},
				{"id": "3", "mission_name": "Trailblazer"},
			}

			resp := map[string]interface{}{"data": map[string]interface{}{"launchesPast": launches}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Unknown query
		http.Error(w, "unknown query", http.StatusBadRequest)
	})
	log.Printf("Mock GraphQL server running at %s/graphql", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("mock server failed: %v", err)
	}
}

func main() {
	// Spin up the mock server in the background
	go startMockServer(":8080")

	// Give it a moment to start
	time.Sleep(200 * time.Millisecond)

	// 1) Initialize a new GraphQL transport with a logger
	transport := utcp.NewGraphQLClientTransport(func(msg string, err error) {
		if err != nil {
			log.Printf("[GraphQL][ERROR] %s: %v", msg, err)
		} else {
			log.Printf("[GraphQL] %s", msg)
		}
	})
	defer func() {
		if err := transport.Close(); err != nil {
			log.Printf("Error closing transport: %v", err)
		}
	}()

	// 2) Define the GraphQL endpoint and optional headers/auth
	provider := &providers.GraphQLProvider{
		URL:     "http://localhost:8080/graphql", // point to local mock server
		Headers: map[string]string{"X-Custom-Header": "example"},
		Auth:    nil, // no authentication for this example
	}

	// 3) Create a context with timeout for all GraphQL operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 4) Discover available operations (queries & mutations)
	tools, err := transport.RegisterToolProvider(ctx, provider)
	if err != nil {
		log.Fatalf("failed to register GraphQL provider: %v", err)
	}

	// Print discovered endpoints
	fmt.Println("Discovered GraphQL operations:")
	for _, t := range tools {
		fmt.Printf("  • %s: %s\n", t.Name, t.Description)
	}

	// 5) Execute a sample query: fetch the last 3 past SpaceX launches
	variables := map[string]any{
		"limit": "3",
	}
	result, err := transport.CallTool(ctx, "launchesPast", variables, provider, nil)
	if err != nil {
		log.Fatalf("GraphQL query failed: %v", err)
	}

	// 6) Handle and display the result
	launches, ok := result.([]interface{})
	if !ok {
		log.Fatalf("unexpected result type: %T", result)
	}
	fmt.Println("Last 3 past SpaceX launches:")
	for i, l := range launches {
		launch := l.(map[string]interface{})
		exid, _ := launch["id"]
		name, _ := launch["mission_name"]
		fmt.Printf("  %d. %s (ID: %s)\n", i+1, name, exid)
	}
}
