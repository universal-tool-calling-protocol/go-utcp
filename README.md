# UTCP-Golang
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
    "log"

    utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

func main() {
    ctx := context.Background()

    client, err := utcp.NewUTCPClient(ctx, nil, nil, nil)
    if err != nil {
        log.Fatalf("create client: %v", err)
    }

    tools, err := client.SearchTools(ctx, "", 10)
    if err != nil {
        log.Fatalf("search: %v", err)
    }

    if len(tools) > 0 {
        if _, err := client.CallTool(ctx, tools[0].Name, nil); err != nil {
            log.Fatalf("call: %v", err)
        }
    }
}
```

The library is primarily intended for experimentation and
interoperability testing.  The API may change without notice.
