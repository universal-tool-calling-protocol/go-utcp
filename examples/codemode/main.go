package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Protocol-Lattice/go-agent/src/adk"
	adkmodules "github.com/Protocol-Lattice/go-agent/src/adk/modules"
	"github.com/Protocol-Lattice/go-agent/src/memory"
	"github.com/Protocol-Lattice/go-agent/src/memory/engine"
	"github.com/Protocol-Lattice/go-agent/src/models"
	"github.com/Protocol-Lattice/go-agent/src/subagents"
	"github.com/universal-tool-calling-protocol/go-utcp"
)

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

		// 1. Discovery request (empty body, first call)
		if len(raw) == 0 && !discovered {
			discovered = true
			data, err := os.ReadFile("tools.json")
			if err != nil {
				log.Printf("Failed to read tools.json: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(data)
			return
		}

		// 2. Parse incoming JSON (any format)
		var payload map[string]interface{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		// ------------------------------------------------------------
		// SUPPORTED UTCP FORMATS (real ones used by Go ADK in Nov 2025)
		// ------------------------------------------------------------

		// Format A: Modern official UTCP → { "name": "tool.name", "input": { ... } }
		if toolName, ok := payload["name"].(string); ok {
			input := map[string]interface{}{}
			if inp, ok := payload["input"]; ok {
				if m, ok := inp.(map[string]interface{}); ok {
					input = m
				}
			}
			handleTool(w, toolName, input)
			return
		}

		// Fallback: Use tool name from path and payload as input
		if toolName := strings.TrimPrefix(r.URL.Path, "/tools"); toolName != "" {
			handleTool(w, toolName, payload)
			return
		}

		// Format D: Some older experimental clients → { "tool": "...", "args": { ... } }
		if toolName, ok := payload["tool"].(string); ok {
			args := map[string]interface{}{}
			if a, ok := payload["args"]; ok {
				if m, ok := a.(map[string]interface{}); ok {
					args = m
				}
			}
			handleTool(w, toolName, args)
			return
		}

		// If we get here → unknown format
		log.Printf("Unknown request format: %v", payload)
		http.Error(w, "unknown request format", http.StatusBadRequest)
	})

	log.Printf("HTTP mock server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// Central tool handler (all formats end up here)
func handleTool(w http.ResponseWriter, toolName string, args map[string]interface{}) {
	log.Printf("Tool call → %s | args: %v", toolName, args)

	switch toolName {

	case "http.echo":
		msg := fmt.Sprintf("%v", args["message"])
		json.NewEncoder(w).Encode(map[string]any{"result": msg})

	case "http.timestamp":
		format := "2006-01-02T15:04:05Z07:00"
		if f, ok := args["format"].(string); ok && f != "" {
			format = f
		}
		json.NewEncoder(w).Encode(map[string]any{"result": time.Now().Format(format)})

	case "http.math.add":
		a := toFloat(args["a"])
		b := toFloat(args["b"])
		if a == nil || b == nil {
			http.Error(w, "missing or invalid 'a' or 'b'", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"result": *a + *b})

	case "http.math.multiply":
		a := toFloat(args["a"])
		b := toFloat(args["b"])
		if a == nil || b == nil {
			http.Error(w, "missing or invalid 'a' or 'b'", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"result": *a * *b})

	case "http.string.concat":
		// The tool call was sending 'a' and 'b' but the handler expected 'prefix' and 'value'.
		// This is now aligned with the tools.json spec.
		prefix := fmt.Sprintf("%v", args["prefix"])
		value := fmt.Sprintf("%v", args["value"])
		json.NewEncoder(w).Encode(map[string]any{"result": prefix + value})

	case "http.stream.echo":
		// Simple non-streaming echo for testing (streaming not required for this demo)
		input := fmt.Sprintf("%v", args["input"])
		json.NewEncoder(w).Encode(map[string]any{"result": input})

	default:
		log.Printf("Unknown tool: %s", toolName)
		http.Error(w, fmt.Sprintf("unknown tool: %s", toolName), http.StatusBadRequest)
	}
}

// Helper to safely convert interface{} → float64
func toFloat(v interface{}) *float64 {
	if v == nil {
		return nil
	}
	switch n := v.(type) {
	case float64:
		return &n
	case float32:
		f := float64(n)
		return &f
	case int:
		f := float64(n)
		return &f
	case int64:
		f := float64(n)
		return &f
	default:
		return nil
	}
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

	modelName := flag.String("model", "gemini-2.5-flash", "Gemini model ID")
	flag.Parse()

	researcherModel, err := models.NewGeminiLLM(ctx, *modelName, "")
	if err != nil {
		log.Fatalf("failed to create researcher model: %v", err)
	}

	memOpts := engine.DefaultOptions()
	kit, err := adk.New(ctx,
		adk.WithDefaultSystemPrompt(""),
		adk.WithSubAgents(subagents.NewResearcher(researcherModel)),
		adk.WithModules(
			adkmodules.NewModelModule("gemini-model", func(_ context.Context) (models.Agent, error) {
				return models.NewGeminiLLM(ctx, *modelName, "")
			}),
			adkmodules.InMemoryMemoryModule(100000, memory.AutoEmbedder(), &memOpts),
		),
		adk.WithCodeModeUtcp(client),
		adk.WithUTCP(client),
	)
	if err != nil {
		log.Fatalf("failed to initialise kit: %v", err)
	}

	ag, err := kit.BuildAgent(ctx)
	if err != nil {
		log.Fatalf("failed to build agent: %v", err)
	}

	prompt := `
1. Call the http.echo tool with the message "hello world".
2. Call the http.math.add tool with a=5 and b=7.
3. Take the result from the previous step and call the http.math.multiply tool with it and the number 3.
4. Take the result from the previous step and call the http.string.concat tool to prepend "Number: " to it, using 'prefix' and 'value' as arguments.
`

	resp, err := ag.Generate(ctx, "test", prompt)
	if err != nil {
		log.Fatalf("failed to generate: %v", err)
	}
	fmt.Println(resp)
}
