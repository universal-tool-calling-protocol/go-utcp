package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

// discovered flags whether we've serviced the UTCP discovery call yet
var discovered bool

func startServer(addr string) {
	http.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to read body: %v", err), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		log.Printf("Received raw request: %s", string(raw))

		// Discovery: first empty-body => discovery
		if len(raw) == 0 && !discovered {
			discovered = true
			// Read discovery response from tools.json
			data, err := os.ReadFile("tools.json")
			if err != nil {
				log.Printf("Failed to read tools.json: %v", err)
				http.Error(w, fmt.Sprintf("failed to read tools.json: %v", err), http.StatusInternalServerError)
				return
			}
			var discoveryResponse map[string]interface{}
			if err := json.Unmarshal(data, &discoveryResponse); err != nil {
				log.Printf("Failed to unmarshal tools.json: %v", err)
				http.Error(w, fmt.Sprintf("invalid tools.json format: %v", err), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(discoveryResponse); err != nil {
				log.Printf("Failed to encode discovery response: %v", err)
			}
			return
		}

		// Empty-body after discovery => timestamp call
		if len(raw) == 0 {
			log.Printf("Empty body – timestamp call")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"result": time.Now().Format(time.RFC3339)})
			return
		}

		// Try to parse the JSON
		var probe map[string]interface{}
		if err := json.Unmarshal(raw, &probe); err != nil {
			http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
			return
		}

		// Standard tool call (has "tool" field)
		if toolName, hasToolField := probe["tool"].(string); hasToolField && toolName != "" {
			var req struct {
				Tool string                 `json:"tool"`
				Args map[string]interface{} `json:"args"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				http.Error(w, fmt.Sprintf("invalid JSON for tool call: %v", err), http.StatusBadRequest)
				return
			}

			log.Printf("Standard tool call: %s with args: %v", req.Tool, req.Args)
			w.Header().Set("Content-Type", "application/json")

			switch req.Tool {
			case "echo":
				msg, _ := req.Args["message"].(string)
				json.NewEncoder(w).Encode(map[string]any{"result": msg})
			case "timestamp":
				json.NewEncoder(w).Encode(map[string]any{"result": time.Now().Format(time.RFC3339)})
			default:
				http.Error(w, "unknown tool", http.StatusNotFound)
			}
			return
		}

		// Direct echo call (has "message" field)
		if _, hasMessage := probe["message"]; hasMessage {
			log.Printf("Direct echo call with args: %v", probe)
			w.Header().Set("Content-Type", "application/json")
			msg, _ := probe["message"].(string)
			json.NewEncoder(w).Encode(map[string]any{"result": msg})
			return
		}

		// Unknown request format
		log.Printf("Unknown request format: %v", probe)
		http.Error(w, "unknown request format", http.StatusBadRequest)
	})

	log.Printf("HTTP mock server on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func main() {
	go startServer(":8080")
	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("client error: %v", err)
	}

	// Discover tools
	tools, err := client.SearchTools("", 10)
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	fmt.Printf("Discovered %d tools:\n", len(tools))
	for _, t := range tools {
		fmt.Printf(" - %s\n", t.Name)
	}

	// Call the "echo" tool
	res, err := client.CallTool(ctx, "http.echo", map[string]any{"message": "hi"})
	if err != nil {
		log.Fatalf("call echo: %v", err)
	}
	fmt.Printf("Echo result: %#v\n", res)

	// Call the "timestamp" tool
	ts, err := client.CallTool(ctx, "http.timestamp", map[string]any{})
	if err != nil {
		log.Fatalf("call timestamp: %v", err)
	}
	fmt.Printf("Timestamp result: %#v\n", ts)
}
