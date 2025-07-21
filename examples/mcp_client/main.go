package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

func startServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/utcp", func(w http.ResponseWriter, r *http.Request) {
		manual := utcp.UtcpManual{
			Version: utcp.Version,
			Tools:   []utcp.Tool{{Name: "hello", Description: "Say hello"}},
		}
		_ = json.NewEncoder(w).Encode(manual)
	})
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Name == "" {
			req.Name = "World"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": fmt.Sprintf("Hello, %s!", req.Name)})
	})
	srv := &http.Server{Addr: addr, Handler: mux}
	go srv.ListenAndServe()
	return srv
}

func main() {
	srv := startServer(":8000")
	defer srv.Close()

	time.Sleep(200 * time.Millisecond)

	ctx := context.Background()
	cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "provider.json"}
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		panic(err)
	}

	tools, err := client.SearchTools("", 10)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Discovered %d tools\n", len(tools))
	for _, t := range tools {
		fmt.Printf(" - %s\n", t.Name)
	}
}
