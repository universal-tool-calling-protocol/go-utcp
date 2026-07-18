# go-utcp

![MCP vs. UTCP](https://github.com/universal-tool-calling-protocol/.github/raw/main/assets/banner.png)

[![Go Report Card](https://goreportcard.com/badge/github.com/universal-tool-calling-protocol/go-utcp)](https://goreportcard.com/report/github.com/universal-tool-calling-protocol/go-utcp)
[![Go Reference](https://pkg.go.dev/badge/github.com/universal-tool-calling-protocol/go-utcp.svg)](https://pkg.go.dev/github.com/universal-tool-calling-protocol/go-utcp)
[![License: MPL 2.0](https://img.shields.io/badge/License-MPL_2.0-blue.svg)](LICENSE)

**Discover and call tools over the protocols they already use.**

`go-utcp` is a Go client for the [Universal Tool Calling Protocol (UTCP)](https://github.com/universal-tool-calling-protocol). A provider describes its tools and how to reach them; the client discovers that metadata, indexes it locally, and dispatches calls over the provider's native transport.

The result is one API for tools exposed through HTTP, command-line programs, WebSockets, gRPC, GraphQL, MCP, and other transports.

## Why go-utcp?

- **One client API:** discover, search, call, and stream tools without transport-specific application code.
- **Native connectivity:** use existing APIs and services without moving them behind a single gateway.
- **Local tool catalog:** keep discovered metadata in memory by default, or supply your own repository and search strategy.
- **Flexible configuration:** load providers from JSON, register them at runtime, and inject secrets from explicit variables, `.env` files, or the environment.
- **Broad transport support:** HTTP, CLI, SSE, streamable HTTP, WebSocket, gRPC/gNMI, GraphQL, TCP, UDP, WebRTC, MCP, and local text templates.
- **Tool composition:** use CodeMode to orchestrate multi-step workflows with small Go-like programs.

## Quick start

### Requirements

- Go 1.25 or newer

Install the module:

```sh
go get github.com/universal-tool-calling-protocol/go-utcp@latest
```

Create a `providers.json` file. This example uses the local text transport, so it needs no server or credentials:

```json
{
  "providers": [
    {
      "provider_type": "text",
      "name": "greetings",
      "templates": {
        "hello": "Hello, {{.name}}!"
      }
    }
  ]
}
```

Create `main.go`:

```go
package main

import (
	"context"
	"fmt"
	"log"

	utcp "github.com/universal-tool-calling-protocol/go-utcp"
)

func main() {
	ctx := context.Background()

	client, err := utcp.NewUTCPClient(ctx, &utcp.UtcpClientConfig{
		ProvidersFilePath: "providers.json",
	}, nil, nil)
	if err != nil {
		log.Fatal(err)
	}

	discovered, err := client.SearchTools("", 10)
	if err != nil {
		log.Fatal(err)
	}
	for _, tool := range discovered {
		fmt.Printf("%s: %s\n", tool.Name, tool.Description)
	}

	result, err := client.CallTool(ctx, "greetings.hello", map[string]any{
		"name": "UTCP",
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result)
}
```

Run it:

```sh
go run .
```

The client loads the provider during construction and qualifies every discovered tool as `<provider>.<tool>`:

```text
Successfully registered provider greetings (1 tools)
greetings.hello: Text template tool
Hello, UTCP!
```

## How it works

1. **Configure providers.** Each provider specifies a transport, a name, and the connection details for discovery and invocation.
2. **Discover tools.** `NewUTCPClient` loads configured providers; `RegisterToolProvider` can add more at runtime.
3. **Search locally.** Discovered schemas are stored in the tool repository and searched without contacting every provider again.
4. **Call by qualified name.** `CallTool` and `CallToolStream` resolve the provider and dispatch through the matching transport.

## Provider configuration

`ProvidersFilePath` accepts any of these JSON root shapes:

- An array of provider objects
- A single provider object
- An object whose `providers` field contains an array or one provider object

Every provider requires a `provider_type` (the `type` alias is also accepted). A `name` is strongly recommended because it becomes the prefix for all of that provider's tools.

| `provider_type` | Connects to |
| --- | --- |
| `http` | UTCP manuals or OpenAPI documents over HTTP/HTTPS |
| `cli` | Local command-line processes |
| `sse` | Server-Sent Events endpoints |
| `http_stream` | Streamable HTTP endpoints |
| `websocket` | WebSocket services |
| `grpc` | gRPC services and gNMI telemetry |
| `graphql` | GraphQL queries and subscriptions |
| `tcp` | Raw TCP services |
| `udp` | UDP services |
| `webrtc` | WebRTC data channels |
| `mcp` | MCP servers over stdio or HTTP |
| `text` | Local Go text templates |

Provider-specific fields and runnable client/transport pairs are documented in the [examples guide](examples/README.md).

### Variables and secrets

Any provider string can reference `$NAME` or `${NAME}`. Values are looked up in this order:

1. `UtcpClientConfig.Variables`
2. Loaders in `UtcpClientConfig.LoadVariablesFrom`
3. Process environment variables

```go
cfg := &utcp.UtcpClientConfig{
	ProvidersFilePath: "providers.json",
	Variables: map[string]string{
		"API_HOST": "api.example.com",
	},
	LoadVariablesFrom: []utcp.UtcpVariablesConfig{
		utcp.NewDotEnv(".env"),
	},
}
```

The corresponding provider can keep credentials out of committed JSON:

```json
{
  "provider_type": "http",
  "name": "catalog",
  "http_method": "POST",
  "url": "https://${API_HOST}/tools",
  "headers": {
    "Authorization": "Bearer ${API_TOKEN}"
  }
}
```

## Discover, search, and call

`SearchTools(query, limit)` has three useful modes:

| Query | Behavior |
| --- | --- |
| `""` | Return tools from the entire local catalog |
| Exact provider name | Return only that provider's tools |
| Any other text | Rank tools using the configured search strategy |

The default strategy matches the query against tool tags and description words. A non-positive `limit` returns all matching tools.

The core client interface is deliberately small:

| Method | Purpose |
| --- | --- |
| `RegisterToolProvider` | Discover and store tools from a provider |
| `DeregisterToolProvider` | Remove a provider and its tools |
| `SearchTools` | List, filter, or rank locally indexed tools |
| `CallTool` | Invoke a tool and return its result |
| `CallToolStream` | Invoke a streaming tool and return a `StreamResult` |
| `GetTransports` | Access the registered client transports |

`NewUTCPClient` accepts optional repository and search-strategy arguments. Pass `nil` for the built-in in-memory repository and tag/description search:

```go
client, err := utcp.NewUTCPClient(ctx, cfg, nil, nil)
```

## Streaming results

For transports and tools that support streaming, read from the returned `StreamResult` until `io.EOF` and always close it:

```go
stream, err := client.CallToolStream(ctx, "events.watch", args)
if err != nil {
	log.Fatal(err)
}
defer stream.Close()

for {
	item, err := stream.Next()
	if errors.Is(err, io.EOF) {
		break
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(item)
}
```

Streaming availability depends on the provider transport.

## CodeMode

[CodeMode](src/plugins/codemode/README.md) lets an LLM or application compose registered tools with interpreted Go-like snippets. Snippets can branch, loop, process intermediate values, and use these helpers:

- `codemode.CallTool`
- `codemode.CallToolStream`
- `codemode.SearchTools`

Use `NewCodeModeUTCP` for model-driven orchestration or `Execute` to run a snippet directly. See the [CodeMode guide](src/plugins/codemode/README.md) for the API, execution rules, and examples.

## Examples

The [`examples`](examples/README.md) directory contains standalone modules for every supported client and transport. To run the zero-dependency text client:

```sh
cd examples/text_client
GOWORK=off go run -mod=mod .
```

Some network examples start their own demo service; others are designed to run alongside the corresponding transport example. Check each example's source for its setup.

## Development

```sh
git clone https://github.com/universal-tool-calling-protocol/go-utcp.git
cd go-utcp
go test ./...
```

See [onboarding.md](onboarding.md) for the contributor workflow.

## License

`go-utcp` is licensed under the [Mozilla Public License 2.0](LICENSE).
