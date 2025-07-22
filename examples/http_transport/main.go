package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers"
	utcp "github.com/universal-tool-calling-protocol/go-utcp/src/transports/http"
)

// Tool metadata
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema,omitempty"`
}

// Our in‑memory tool list
var tools = []Tool{
	{
		Name:        "echo",
		Description: "Returns back the message you send it.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"message": map[string]string{"type": "string"}},
			"required":   []string{"message"},
		},
	},
	{
		Name:        "timestamp",
		Description: "Returns the current server timestamp in RFC3339.",
	},
}

func main() {
	// 1) Start the HTTP server in a goroutine
	go startToolServer(":8080")

	// 2) Give the server a moment to come up
	time.Sleep(200 * time.Millisecond)

	// 3) Run the client that discovers & calls "echo"
	runClient("http://localhost:8080/tools")
}

// startToolServer boots the HTTP API that lists & invokes tools.
func startToolServer(addr string) {
	r := mux.NewRouter()
	r.HandleFunc("/tools", listToolsHandler).Methods("GET")
	r.HandleFunc("/tools/{name}/call", callToolHandler).Methods("POST")

	srv := &http.Server{
		Handler:      r,
		Addr:         addr,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("🔧 Tool provider listening on %s …", addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func listToolsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Return in UTCP manual format for proper tool discovery
	response := map[string]interface{}{
		"version": "1.0",
		"tools":   tools,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func callToolHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	var args map[string]interface{}

	// Handle empty body for tools that don't need arguments
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		args = make(map[string]interface{})
	}

	var result interface{}
	var err error

	switch name {
	case "echo":
		msg, ok := args["message"].(string)
		if !ok {
			err = fmt.Errorf("missing or invalid 'message'")
		} else {
			result = map[string]string{"echo": msg}
		}
	case "timestamp":
		result = map[string]string{"timestamp": time.Now().Format(time.RFC3339)}
	default:
		http.Error(w, "unknown tool: "+name, http.StatusNotFound)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"result": result})
}

// runClient demonstrates UTCP discovering tools and calling both "echo" and "timestamp".
func runClient(baseURL string) {
	logger := func(format string, args ...interface{}) {
		log.Printf(format, args...)
	}

	transport := utcp.NewHttpClientTransport(logger)

	// Provider for tool discovery
	discoveryProvider := &providers.HttpProvider{
		URL:        baseURL,
		HTTPMethod: "GET",
		Headers:    map[string]string{"Accept": "application/json"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Discover tools
	tools, err := transport.RegisterToolProvider(ctx, discoveryProvider)
	if err != nil {
		log.Fatalf("Failed to register provider: %v", err)
	}
	log.Printf("Discovered %d tools:", len(tools))
	for _, t := range tools {
		log.Printf(" • %s: %s", t.Name, t.Description)
	}

	// Provider for tool calling (different URL pattern and POST method)
	callProvider := &providers.HttpProvider{
		URL:        "http://localhost:8080/tools/echo/call",
		HTTPMethod: "POST",
		Headers:    map[string]string{"Content-Type": "application/json"},
	}

	// Call "echo" tool
	args := map[string]interface{}{"message": "Hello from Go!"}
	result, err := transport.CallTool(ctx, "echo", args, callProvider, nil)
	if err != nil {
		log.Fatalf("CallTool error: %v", err)
	}
	fmt.Printf("✅ Echo tool response: %#v\n", result)

	// Call "timestamp" tool (send empty JSON object)
	timestampProvider := &providers.HttpProvider{
		URL:        "http://localhost:8080/tools/timestamp/call",
		HTTPMethod: "POST",
		Headers:    map[string]string{"Content-Type": "application/json"},
	}

	timestampResult, err := transport.CallTool(ctx, "timestamp", map[string]interface{}{}, timestampProvider, nil)
	if err != nil {
		log.Fatalf("CallTool timestamp error: %v", err)
	}
	fmt.Printf("✅ Timestamp tool response: %#v\n", timestampResult)
}
