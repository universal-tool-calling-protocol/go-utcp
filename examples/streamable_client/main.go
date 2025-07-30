package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
	"github.com/universal-tool-calling-protocol/go-utcp/src/transports"
)

func startStreamingServer(addr string) {
	// Tools discovery endpoint - returns available tools
	http.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		tools := struct {
			Tools []Tool `json:"tools"`
		}{
			Tools: []Tool{
				{
					Name:        "streamNumbers",
					Description: "Streams numbers from 1 to 5",
					Inputs: ToolInputOutputSchema{
						Type:       "object",
						Properties: map[string]interface{}{},
					},
					Outputs: ToolInputOutputSchema{
						Type: "object",
						Properties: map[string]interface{}{
							"number": map[string]interface{}{
								"type":        "integer",
								"description": "A streamed number",
							},
						},
					},
				},
			},
		}

		json.NewEncoder(w).Encode(tools)
	})

	// Actual streaming endpoint - the client requests with provider name prefix
	http.HandleFunc("/tools/http_stream.streamNumbers", func(w http.ResponseWriter, r *http.Request) {
		// Log the request for debugging
		log.Printf("Received %s request to %s", r.Method, r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		for i := 1; i <= 5; i++ {
			obj := map[string]int{"number": i}
			if data, err := json.Marshal(obj); err == nil {
				fmt.Fprint(w, string(data), "\n")
				flusher.Flush()
			}
			time.Sleep(200 * time.Millisecond)
		}
	})

	// Also add a catch-all handler to see what requests are coming in
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Unhandled request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})

	log.Printf("Streaming server on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func main() {
	go startStreamingServer(":8080")
	time.Sleep(500 * time.Millisecond) // Give server more time to start

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

	res, err := client.CallTool(ctx, "http_stream.streamNumbers", nil, true)
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
