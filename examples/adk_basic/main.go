package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/universal-tool-calling-protocol/go-utcp/src/adk"
	base "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

func main() {
	agent := adk.NewAgent(
		"adk-basic",
		adk.WithDescription("Example agent built with the UTCP ADK"),
		adk.WithDefaultProvider(&base.BaseProvider{Name: "ADK HTTP Provider", ProviderType: base.ProviderHTTP}),
	)

	agent.MustRegisterTool(adk.ToolDefinition{
		Name:        "greet",
		Description: "Returns a friendly greeting",
		Tags:        []string{"example", "adk"},
		Handler: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			name, _ := input["name"].(string)
			if name == "" {
				name = "world"
			}
			return map[string]any{"greeting": "Hello " + name + "!"}, nil
		},
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Println("Serving UTCP agent on http://localhost:8080 (manual available at /manual)")
	if err := agent.ServeHTTP(ctx, ":8080"); err != nil {
		log.Fatalf("agent server exited with error: %v", err)
	}
	log.Println("Agent server stopped")
}
