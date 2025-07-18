# UTCP-Golang
[![Go Report Card](https://goreportcard.com/badge/github.com/universal-tool-calling-protocol/UTCP)](https://goreportcard.com/report/github.com/universal-tool-calling-protocol/UTCP)

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

The library is primarily intended for experimentation and
interoperability testing.  The API may change without notice.
