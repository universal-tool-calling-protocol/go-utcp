package main

import (
	"context"
	"fmt"
	"log"

	utcp "github.com/Raezil/UTCP"
)

func main() {
	// Initialize logger
	logger := func(msg string, err error) {
		if err != nil {
			log.Printf("[GraphQLTransport] %s: %v", msg, err)
		} else {
			log.Printf("[GraphQLTransport] %s", msg)
		}
	}

	// Create GraphQL transport
	transport := utcp.NewGraphQLClientTransport(logger)
	defer transport.Close()

	ctx := context.Background()

	// Configure your GraphQL provider
	prov := &utcp.GraphQLProvider{
		URL:     "https://api.spacex.land/graphql/",
		Headers: map[string]string{"Content-Type": "application/json"},
		// Auth: nil, // No auth needed for this public API
	}

	// Discover available operations (queries & mutations)
	tools, err := transport.RegisterToolProvider(ctx, prov)
	if err != nil {
		log.Fatalf("failed to register provider: %v", err)
	}

	// Print discovered operations
	fmt.Println("Available operations:")
	for _, t := range tools {
		fmt.Printf("- %s: %s\n", t.Name, t.Description)
	}

	// Example: call a query, e.g. "launchesPast"
	exampleArgs := map[string]interface{}{
		"limit": "3",
	}
	result, err := transport.CallTool(ctx, "launchesPast", exampleArgs, prov, nil)
	if err != nil {
		log.Fatalf("error calling launchesPast: %v", err)
	}

	// Print the result
	fmt.Printf("\nResult of launchesPast: %+v\n", result)
}
