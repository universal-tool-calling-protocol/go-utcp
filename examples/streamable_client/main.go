package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

func startServer(addr string) {
	http.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
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
	log.Printf("Streaming server on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func main() {
	go startServer(":8080")
	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		log.Fatalf("client: %v", err)
	}

	res, err := client.CallTool(ctx, "stream.streamNumbers", nil)
	if err != nil {
		log.Fatalf("call: %v", err)
	}
	fmt.Printf("Result: %#v\n", res)
}
