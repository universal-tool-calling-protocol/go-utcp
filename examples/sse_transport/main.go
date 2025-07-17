package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Raezil/UTCP"
)

func main() {

	// Optional: SSE transport example (ensure URL is a valid SSE endpoint streaming JSON)
	// Uncomment and configure the following block if you have a working SSE provider.

	// SSE example
	ctx := context.Background()
	sseLogger := func(format string, args ...interface{}) {
		fmt.Printf("[SSE] "+format+"\n", args...)
	}
	sseTransport := UTCP.NewSSETransport(sseLogger)
	sseProvider := &UTCP.SSEProvider{
		URL:     "https://your-sse-endpoint.example.com/stream",
		Headers: map[string]string{"Authorization": "Bearer YOUR_TOKEN"},
	}
	//
	tools, err := sseTransport.RegisterToolProvider(ctx, sseProvider)
	if err != nil {
		panic(fmt.Errorf("failed to register SSE tools: %w", err))
	}
	fmt.Println("SSE tools discovered:")
	for _, t := range tools {
		fmt.Printf(" - %s: %s\n", t.Name, t.Description)
	}

	// Ensure logs flush before exit
	time.Sleep(500 * time.Millisecond)
}
