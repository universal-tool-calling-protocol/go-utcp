package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	UTCP "github.com/universal-tool-calling-protocol/go-utcp"
)

func main() {
	// 1) Start a mock SSE provider locally
	go startMockServer(":8080")

	// 2) Give the server a moment to start
	time.Sleep(200 * time.Millisecond)

	// 3) Run the UTCP client against the local provider
	runClient("http://localhost:8080")
}

// startMockServer boots a simple HTTP API that mimics an SSE provider.
func startMockServer(addr string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"version": "1.0",
			"tools": []map[string]interface{}{
				{"name": "hello", "description": "Returns a greeting"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&in)
		name, _ := in["name"].(string)

		// Check if client requested SSE
		if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
				return
			}

			// Stream two parts of the greeting
			parts := []map[string]string{{"result": "Hello,"}, {"result": fmt.Sprintf(" %s!", name)}}
			for _, part := range parts {
				b, _ := json.Marshal(part)
				fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
				flusher.Flush()
				time.Sleep(100 * time.Millisecond)
			}
			return
		}

		// Fallback JSON
		out := map[string]interface{}{"result": fmt.Sprintf("Hello, %s!", name)}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})

	log.Printf("Mock SSE provider listening on %s...", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// runClient demonstrates registering and calling a tool via the SSE transport.
func runClient(baseURL string) {
	ctx := context.Background()
	logger := func(format string, args ...interface{}) {
		fmt.Printf("[SSE] "+format+"\n", args...)
	}
	transport := UTCP.NewSSETransport(logger)

	// Discovery endpoint
	provider := &UTCP.SSEProvider{URL: baseURL + "/tools"}
	tools, err := transport.RegisterToolProvider(ctx, provider)
	if err != nil {
		panic(fmt.Errorf("failed to register SSE tools: %w", err))
	}
	fmt.Println("SSE tools discovered:")
	for _, t := range tools {
		fmt.Printf(" - %s: %s\n", t.Name, t.Description)
	}

	// Update URL for tool calls
	provider.URL = baseURL
	// Call with streaming
	res, err := transport.CallTool(ctx, "hello", map[string]interface{}{"name": "UTCP"}, provider, nil)
	if err != nil {
		panic(fmt.Errorf("failed to call tool: %w", err))
	}

	// Print streaming result
	switch ev := res.(type) {
	case []interface{}:
		fmt.Println("Streamed tool response:")
		for i, chunk := range ev {
			fmt.Printf(" chunk %d: %#v\n", i+1, chunk)
		}
	default:
		fmt.Printf("Tool response: %#v\n", ev)
	}

	// Ensure logs flush before exit
	time.Sleep(500 * time.Millisecond)
}
