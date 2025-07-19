package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

func startServer(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"version": "1.0",
			"tools":   []map[string]interface{}{{"name": "hello", "description": "Returns a greeting"}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&in)
		name, _ := in["name"].(string)
		if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			parts := []map[string]string{{"result": "Hello,"}, {"result": fmt.Sprintf(" %s!", name)}}
			for _, p := range parts {
				b, _ := json.Marshal(p)
				fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
				flusher.Flush()
				time.Sleep(100 * time.Millisecond)
			}
			return
		}
		out := map[string]interface{}{"result": fmt.Sprintf("Hello, %s!", name)}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})
	log.Printf("SSE server on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
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

	res, err := client.CallTool(ctx, "sse.hello", map[string]any{"name": "UTCP"})
	if err != nil {
		log.Fatalf("call: %v", err)
	}
	fmt.Printf("Result: %#v\n", res)
}
