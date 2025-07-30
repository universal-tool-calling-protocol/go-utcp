# go-utcp

[![Go Report Card](https://goreportcard.com/badge/github.com/universal-tool-calling-protocol/go-utcp)](https://goreportcard.com/report/github.com/universal-tool-calling-protocol/go-utcp)
[![Go Reference](https://pkg.go.dev/badge/github.com/universal-tool-calling-protocol/go-utcp.svg)](https://pkg.go.dev/github.com/universal-tool-calling-protocol/go-utcp)

`go-utcp` is a Go implementation of the **Universal Tool Calling Protocol (UTCP)**. The library allows applications to discover and invoke tools exposed over a variety of transports such as HTTP, CLI, gRPC, WebSocket and more.

UTCP aims to provide a unified way to describe and call tools regardless of how they are hosted. Each provider exposes a "manual" describing the available tools and the client loads these manuals and uses the appropriate transport to perform the calls.

## Features

- Built-in transports for HTTP, CLI, Server-Sent Events, streaming HTTP, GraphQL, gRPC, TCP, UDP, WebRTC, WebSocket and MCP.
- Variable substitution via environment variables or `.env` files using `UtcpDotEnv`.
- In-memory repository for storing providers and discovered tools.
- `OpenApiConverter` utility to convert OpenAPI definitions into UTCP manuals.
- Numerous example programs under `examples/` demonstrating the different transports.

## Installation

```sh
go get github.com/universal-tool-calling-protocol/go-utcp@latest
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "time"

    utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

func main() {
    ctx := context.Background()
    cfg := &utcp.UtcpClientConfig{ProvidersFilePath: "providers.json"}

    client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
    if err != nil {
        panic(err)
    }

    // Wait for the client to fully initialise
    time.Sleep(500 * time.Millisecond)

    tools, err := client.SearchTools("", 10)
    if err != nil || len(tools) == 0 {
        panic("no tools found")
    }

    result, err := client.CallTool(ctx, tools[0].Name, map[string]any{"name": "UTCP"})
    if err != nil {
        panic(err)
    }
    fmt.Println(result)
}
```

## Examples

Each subdirectory under `examples/` is a standalone Go module. Disable the Go workspace when building an example so that it uses its own `go.mod`:

```sh
GOWORK=off go run ./examples/cli_transport
```

## Project Layout

```
src/            Core library packages
examples/       Example programs for each transport
scripts/        Helper scripts used by tests and examples
```

Additional information about the project can be found in [docs/overview.md](docs/overview.md).

## Development

Run the test suite with:

```sh
go test ./...
```

Pull requests are welcome! Feel free to open issues or propose improvements.

## License

This project is licensed under the terms of the Mozilla Public License 2.0. See the [LICENSE](LICENSE) file for details.
