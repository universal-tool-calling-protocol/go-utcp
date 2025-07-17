package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Raezil/UTCP" // where your streamable_transport.go lives
)

func main() {
	logger := func(format string, args ...interface{}) {
		log.Printf(format, args...)
	}

	transport := UTCP.NewStreamableHTTPTransport(logger)

	// Use the UTCP-defined provider type so the type-assertion passes:
	provider := &UTCP.StreamableHttpProvider{
		URL: "https://api.example.com/tools",
		Headers: map[string]string{
			"Authorization":   "Bearer <YOUR_TOKEN>",
			"X-Custom-Header": "foobar",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tools, err := transport.RegisterToolProvider(ctx, provider)
	if err != nil {
		log.Fatalf("failed to register tool provider: %v", err)
	}
	fmt.Println("Discovered tools:")
	for _, t := range tools {
		fmt.Printf(" • %s: %s\n", t.Name, t.Description)
	}

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
