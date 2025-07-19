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

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

func startServer(addr string) {
	http.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "only POST", http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}
		_ = json.Unmarshal(body, &req)
		if strings.Contains(req.Query, "__schema") {
			schema := map[string]interface{}{
				"__schema": map[string]interface{}{
					"queryType": map[string]interface{}{
						"name":   "Query",
						"fields": []map[string]interface{}{{"name": "launchesPast"}},
					},
					"mutationType": nil,
				},
			}
			resp := map[string]interface{}{"data": schema}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if strings.Contains(req.Query, "launchesPast") {
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
		http.Error(w, "unknown query", http.StatusBadRequest)
	})
	log.Printf("GraphQL server on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func main() {
	go startServer(":8080")
	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("client: %v", err)
	}

	tools, err := client.SearchTools(ctx, "", 10)
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	fmt.Printf("Discovered %d tools:\n", len(tools))
	for _, t := range tools {
		fmt.Printf(" - %s\n", t.Name)
	}

	res, err := client.CallTool(ctx, "launchesPast", map[string]any{"limit": "3"})
	if err != nil {
		log.Fatalf("call: %v", err)
	}
	fmt.Printf("Result: %#v\n", res)
}
