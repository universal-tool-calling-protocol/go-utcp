# go-utcp

![MCP vs. UTCP](https://github.com/universal-tool-calling-protocol/.github/raw/main/assets/banner.png)

[![Go Report Card](https://goreportcard.com/badge/github.com/universal-tool-calling-protocol/go-utcp)](https://goreportcard.com/report/github.com/universal-tool-calling-protocol/go-utcp)
[![Go Reference](https://pkg.go.dev/badge/github.com/universal-tool-calling-protocol/go-utcp.svg)](https://pkg.go.dev/github.com/universal-tool-calling-protocol/go-utcp)
[![License: MPL 2.0](https://img.shields.io/badge/License-MPL_2.0-blue.svg)](LICENSE)

`go-utcp` is a Go client for the [Universal Tool Calling Protocol (UTCP)](https://github.com/universal-tool-calling-protocol). It discovers tools from configured providers, keeps their metadata in a local repository, and exposes one API for searching, calling, and streaming tool results across different transports.

## Features

- One client API for tool discovery, invocation, and streaming.
- Provider configuration through JSON or Go values.
- Built-in HTTP, CLI, SSE, streamable HTTP, WebSocket, gRPC, GraphQL, TCP, UDP, WebRTC, MCP, and local text transports.
- OpenAPI discovery for HTTP providers.
- Runtime provider registration and deregistration.
- `$VAR` and `${VAR}` substitution from explicit values, `.env` files, or the process environment.
- An in-memory tool repository, with interfaces for custom repositories and search strategies.
- CodeMode for composing several tool calls with a small Go-like program.

## Requirements

- Go 1.25 or newer

## Install

```sh
go get github.com/universal-tool-calling-protocol/go-utcp@latest
```

## Quick start

Create `providers.json` with a local text-template provider. This transport needs no external service:

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

Load the provider, inspect its tools, and call `greetings.hello`:

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

	tools, err := client.SearchTools("", 10)
	if err != nil {
		log.Fatal(err)
	}
	for _, tool := range tools {
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

```sh
go run .
```

Tools are qualified as `<provider>.<tool>`, which prevents collisions when several providers expose the same tool name.

## Configure providers

`ProvidersFilePath` accepts any of these JSON root shapes:

- An array of provider objects.
- A single provider object.
- An object with a `providers` array or object.

Every provider needs a `provider_type`. The built-in values are:

| Provider type | Transport |
| --- | --- |
| `http` | HTTP/HTTPS, including OpenAPI discovery |
| `cli` | Local command-line process |
| `sse` | Server-Sent Events |
| `http_stream` | Streamable HTTP |
| `websocket` | WebSocket |
| `grpc` | gRPC and gNMI |
| `graphql` | GraphQL queries and subscriptions |
| `tcp` | Raw TCP |
| `udp` | UDP |
| `webrtc` | WebRTC data channel |
| `mcp` | MCP over stdio or HTTP |
| `text` | Local Go text templates |

Provider-specific fields and complete client/server pairs are available in [`examples/`](examples/README.md).

### Variables and secrets

Provider strings can reference `$NAME` or `${NAME}`. Values are resolved in this order:

1. `UtcpClientConfig.Variables`
2. Entries in `UtcpClientConfig.LoadVariablesFrom`
3. Process environment variables

For example:

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

Keep secrets outside committed provider files.

## Client API

| Method | Purpose |
| --- | --- |
| `RegisterToolProvider` | Discover and store the tools exposed by a provider. |
| `DeregisterToolProvider` | Remove a provider and its tools. |
| `SearchTools` | List all tools or filter them by provider prefix. |
| `CallTool` | Invoke a tool and return its result. |
| `CallToolStream` | Invoke a streaming tool and return a `StreamResult`. |
| `GetTransports` | Access the client's registered transport implementations. |

Pass an empty prefix to `SearchTools` to list every registered tool, or pass a provider name to restrict the result to that provider.

### Stream results

Streaming calls return a `StreamResult`. Read until `io.EOF` and always close the stream:

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

Streaming support depends on the selected transport and tool.

## CodeMode

CodeMode executes a constrained Go-like snippet with helpers for calling registered UTCP tools. It is useful for loops, branching, multi-tool composition, and processing intermediate results without serializing every step as a separate tool request.

```go
import "github.com/universal-tool-calling-protocol/go-utcp/src/plugins/codemode"

cm := codemode.NewCodeModeUTCP(client, nil)
result, err := cm.Execute(ctx, codemode.CodeModeArgs{
	Code: `
		value, err := codemode.CallTool("greetings.hello", map[string]any{
			"name": "CodeMode",
		})
		if err != nil {
			__out = err
			return
		}
		__out = value
	`,
	Timeout: 5_000,
})
```

Snippets can use `codemode.CallTool`, `codemode.CallToolStream`, and `codemode.SearchTools`. `CodeModeResult` contains the resulting value plus captured standard output and standard error.

## Examples

The [`examples`](examples/README.md) directory contains standalone modules for each client and transport. Run an example from its directory with workspace mode disabled so that Go uses the example's own module:

```sh
cd examples/text_client
GOWORK=off go run -mod=mod .
```

Some network examples start an in-process demo server; others require the matching transport example or another local dependency. See the source in each directory for its setup.

## Development

```sh
git clone https://github.com/universal-tool-calling-protocol/go-utcp.git
cd go-utcp
go test ./...
```

New contributors can also follow [`onboarding.md`](onboarding.md).

## License

This project is licensed under the [Mozilla Public License 2.0](LICENSE).
