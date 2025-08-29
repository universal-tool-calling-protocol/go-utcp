# go-utcp
![MCP vs. UTCP](https://github.com/universal-tool-calling-protocol/.github/raw/main/assets/banner.png)

[![Go Report Card](https://goreportcard.com/badge/github.com/universal-tool-calling-protocol/go-utcp)](https://goreportcard.com/report/github.com/universal-tool-calling-protocol/go-utcp)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/universal-tool-calling-protocol/go-utcp)](https://pkg.go.dev/github.com/universal-tool-calling-protocol/go-utcp)
## Introduction

The Universal Tool Calling Protocol (UTCP) is a modern, flexible, and scalable standard for defining and interacting with tools across a wide variety of communication protocols. It is designed to be easy to use, interoperable, and extensible, making it a powerful choice for building and consuming tool-based services.

In contrast to other protocols like MCP, UTCP places a strong emphasis on:

*   **Scalability**: UTCP is designed to handle a large number of tools and providers without compromising performance.
*   **Interoperability**: With support for a wide range of provider types (including HTTP, WebSockets, gRPC, and even CLI tools), UTCP can integrate with almost any existing service or infrastructure.
*   **Ease of Use**: The protocol is built on simple.


![MCP vs. UTCP](https://github.com/user-attachments/assets/3cadfc19-8eea-4467-b606-66e580b89444)



### Features

* Built-in transports for HTTP, CLI, Server-Sent Events, streaming HTTP,
  GraphQL, MCP and UDP.
* Variable substitution via environment variables or `.env` files using
  `UtcpDotEnv`.
* In-memory repository for storing providers and tools discovered at
  runtime.
* Utilities such as `OpenApiConverter` to convert OpenAPI definitions
  into UTCP manuals.
* Example programs demonstrating the client in the `examples` directory.

### Examples

Each subdirectory under `examples/` is a standalone Go module demonstrating a client or transport. For an overview of available examples and usage instructions, see [examples/README.md](examples/README.md). When
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
streaming HTTP, CLI, WebSocket, gRPC, GraphQL, TCP, UDP, WebRTC and MCP providers.

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
