package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
)

func startServer(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		// Open the JSON file
		f, err := os.Open("tools.json")
		if err != nil {

			http.Error(w, "could not load tools.json: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer f.Close()

		// Serve it directly
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.Copy(w, f); err != nil {
			log.Printf("error writing tools.json: %v", err)
		}
	})
	mux.HandleFunc("/tools/sse.hello", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		name, _ := in["name"].(string)
		if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming unsupported", http.StatusBadRequest)
				return
			}
			parts := []string{"Hello,", fmt.Sprintf(" %s!", name)}
			for _, p := range parts {
				b, _ := json.Marshal(map[string]string{"result": p})
				fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
				flusher.Flush()
				time.Sleep(100 * time.Millisecond)
			}
			return
		}
		out := map[string]any{"result": fmt.Sprintf("Hello, %s!", name)}
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
		log.Fatalf("client error: %v", err)
	}

	tools, err := client.SearchTools("", 10)
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	log.Printf("Discovered tools:")
	for _, t := range tools {
		log.Printf(" - %s", t.Name)
	}

	res, err := client.CallTool(ctx, "sse.hello", map[string]any{"name": "UTCP"}, true)
	if err != nil {
		log.Fatalf("call: %v", err)
	}
	sub, ok := res.(transports.StreamResult)
	if !ok {
		log.Fatalf("unexpected subscription type: %T", sub)
	}
	for {
		val, err := sub.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("subscription next error: %v", err)
		}
		log.Printf("Subscription update: %#v", val)
	}
	sub.Close()
}
