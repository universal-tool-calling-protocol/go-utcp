package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/universal-tool-calling-protocol/UTCP"
)

func main() {
	// 1) Start a simple HTTP server that provides tools
	go startToolServer(":8080")
	time.Sleep(200 * time.Millisecond)

	logger := func(format string, args ...interface{}) {
		log.Printf(format, args...)
	}

	transport := UTCP.NewStreamableHTTPTransport(logger)

	// Use the UTCP-defined provider type so the type-assertion passes:
	provider := &UTCP.StreamableHttpProvider{
		URL: "http://localhost:8080/tools",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tools, err := transport.RegisterToolProvider(ctx, provider)
	if err != nil {
		log.Fatalf("failed to register tool provider: %v", err)
	}
	fmt.Println("Discovered tools:")
	for _, t := range tools {
		fmt.Printf(" • %s: %s\n", t.Name, t.Description)
	}

	// Call the translateText tool
	result, err := transport.CallTool(ctx, "translateText", map[string]interface{}{
		"text": "Hello, world!",
		"to":   "es",
	}, provider, nil)
	if err != nil {
		log.Fatalf("tool call failed: %v", err)
	}
	fmt.Printf("Translation result: %#v\n", result)

	if err := transport.DeregisterToolProvider(ctx, provider); err != nil {
		log.Printf("warning: failed to deregister provider: %v", err)
	}
}

// Tool metadata returned by our local server
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var toolList = []Tool{
	{
		Name:        "translateText",
		Description: "Translates text to a target language",
	},
}

func startToolServer(addr string) {
	http.HandleFunc("/tools", listToolsHandler)
	http.HandleFunc("/tools/translateText", callTranslateHandler)

	log.Printf("🔧 Tool provider listening on %s …", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func listToolsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"tools": toolList})
}

func callTranslateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	result := map[string]string{"translatedText": fmt.Sprintf("%s (%s)", req.Text, req.To)}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"result": result})
}
