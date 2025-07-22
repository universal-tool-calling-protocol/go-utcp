package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/universal-tool-calling-protocol/go-utcp/internal/providers"
	utcp "github.com/universal-tool-calling-protocol/go-utcp/internal/transports/streamable"
)

func main() {
	// 1) Start a mock server that streams JSON numbers
	go startStreamingServer(":8080")
	time.Sleep(100 * time.Millisecond) // give it a moment

	// 2) Build your transport
	logger := func(format string, args ...interface{}) {
		log.Printf(format, args...)
	}
	transport := utcp.NewStreamableHTTPTransport(logger)

	// 3) Point at your provider
	provider := &providers.StreamableHttpProvider{
		URL:     "http://localhost:8080/tools",
		Headers: map[string]string{}, // add auth here if needed
	}

	// 4) Call the "streamNumbers" tool
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var lastChunk string
	res, err := transport.CallTool(ctx, "streamNumbers", nil, provider, &lastChunk)
	if err != nil {
		log.Fatalf("CallTool error: %v", err)
	}

	// 5) Inspect what you got
	fmt.Printf("Full result: %#v\n", res)
	fmt.Printf("Last raw chunk: %s\n", lastChunk)
}

// startStreamingServer streams five JSON objects, one every 200ms
func startStreamingServer(addr string) {
	http.HandleFunc("/tools/streamNumbers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
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
	log.Printf("🔧 Streaming tool server on %s…", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
