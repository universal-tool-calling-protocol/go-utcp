# go-utcp
[![Go Report Card](https://goreportcard.com/badge/github.com/universal-tool-calling-protocol/go-utcp)](https://goreportcard.com/report/github.com/universal-tool-calling-protocol/go-utcp)

Universal Tool Calling Protocol (UTCP) reference implementation in Go.

UTCP standardises how a client can discover and call "tools" (APIs)
regardless of the underlying transport.  Each provider describes a
transport (HTTP, CLI, GraphQL and others) and lists the tools it
exposes.  This repository ships a lightweight Go client which can load
provider definitions, register them at runtime and then invoke the
discovered tools.

### Features

* Built-in transports for HTTP, CLI, Server-Sent Events, streaming HTTP,
  GraphQL, MCP, UDP and text-based providers.
* Variable substitution via environment variables or `.env` files using
  `UtcpDotEnv`.
* In-memory repository for storing providers and tools discovered at
  runtime.
* Utilities such as `OpenApiConverter` to convert OpenAPI definitions
  into UTCP manuals.
* Example programs demonstrating the client in the `examples` directory.

### Examples

Each subdirectory under `examples/` is a standalone Go module. When
building or running an example from this repository, disable the
workspace to ensure Go uses the module's own `go.mod`:

```sh
GOWORK=off go run ./examples/cli_transport
```

## Getting Started

Add the library to your project with:

```sh
go get github.com/universal-tool-calling-protocol/go-utcp@latest
```

You can then construct a client and call tools using any of the built-in
transports. The library ships transports for HTTP, Server-Sent Events,
streaming HTTP, CLI, WebSocket, gRPC, GraphQL, TCP, UDP, WebRTC, MCP and
text-based providers.

```go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

func main() {
	ctx := context.Background()

	cfg := &utcp.UtcpClientConfig{
		ProvidersFilePath: "providers.json",
	}

	fmt.Println("Creating UTCP client...")
	client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create UTCP client: %v\n", err)
		os.Exit(1)
	}

	// Give the client time to fully initialize
	fmt.Println("Waiting for initialization...")
	time.Sleep(500 * time.Millisecond)

	fmt.Println("\n=== Tool Discovery ===")
	tools, err := client.SearchTools("", 10)
	if err != nil {
		fmt.Fprintf(os.Stderr, "search error: %v\n", err)
		os.Exit(1)
	}

	if len(tools) == 0 {
		fmt.Println("No tools found!")
		os.Exit(1)
	}

	tool := tools[0]
	fmt.Printf("Found tool: %s\n", tool.Name)
	fmt.Printf("Tool description: %s\n", tool.Description)

	// Test the tool call
	fmt.Println("\n=== Tool Call Test ===")
	input := map[string]interface{}{
		"name": "Kamil",
	}

	fmt.Printf("Calling tool '%s' with input: %v\n", tool.Name, input)
	result, err := client.CallTool(ctx, tool.Name, input)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)

		// Try to understand the error better
		fmt.Printf("Error type: %T\n", err)
		fmt.Printf("Error string: %s\n", err.Error())

		// Let's try a direct search for the provider
		fmt.Println("\n=== Searching for provider directly ===")
		providerTools, err2 := client.SearchTools("hello", 10)
		if err2 != nil {
			fmt.Printf("Provider search failed: %v\n", err2)
		} else {
			fmt.Printf("Provider search returned %d tools\n", len(providerTools))
			for i, t := range providerTools {
				fmt.Printf("  %d: %s\n", i, t.Name)
			}
		}

	} else {
		fmt.Printf("SUCCESS: %v\n", result)
	}
}
```

The library is primarily intended for experimentation and
interoperability testing.  The API may change without notice.
