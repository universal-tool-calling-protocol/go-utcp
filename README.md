# go-utcp

![MCP vs. UTCP](https://github.com/universal-tool-calling-protocol/.github/raw/main/assets/banner.png)

[![Go Report Card](https://goreportcard.com/badge/github.com/universal-tool-calling-protocol/go-utcp)](https://goreportcard.com/report/github.com/universal-tool-calling-protocol/go-utcp)
[![Go Reference](https://pkg.go.dev/badge/github.com/universal-tool-calling-protocol/go-utcp.svg)](https://pkg.go.dev/github.com/universal-tool-calling-protocol/go-utcp)
[![License: MPL 2.0](https://img.shields.io/badge/License-MPL_2.0-blue.svg)](LICENSE)

`go-utcp` is the Go implementation of the [Universal Tool Calling Protocol (UTCP)](https://github.com/universal-tool-calling-protocol). It provides a single client-side abstraction for discovering, describing, searching, invoking, and streaming tools exposed through heterogeneous providers and transports.

The project is designed for applications that need to connect an agent, service, workflow engine, or automation runtime to many kinds of tools without coupling application logic to every transport individually.

## Table of contents

- [What is UTCP?](#what-is-utcp)
- [Why go-utcp?](#why-go-utcp)
- [Core concepts](#core-concepts)
- [Features](#features)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Architecture](#architecture)
- [Providers](#providers)
- [Provider configuration](#provider-configuration)
- [Variables and secrets](#variables-and-secrets)
- [Tool discovery](#tool-discovery)
- [Searching tools](#searching-tools)
- [Calling tools](#calling-tools)
- [Streaming](#streaming)
- [Dynamic provider registration](#dynamic-provider-registration)
- [Transport matrix](#transport-matrix)
- [HTTP](#http)
- [OpenAPI](#openapi)
- [CLI](#cli)
- [SSE](#sse)
- [Streamable HTTP](#streamable-http)
- [WebSocket](#websocket)
- [gRPC](#grpc)
- [GraphQL](#graphql)
- [TCP](#tcp)
- [UDP](#udp)
- [WebRTC](#webrtc)
- [MCP](#mcp)
- [Text providers](#text-providers)
- [CodeMode](#codemode)
- [Security](#security)
- [Error handling](#error-handling)
- [Context cancellation](#context-cancellation)
- [Testing](#testing)
- [Examples](#examples)
- [Development](#development)
- [Project layout](#project-layout)
- [Performance](#performance)
- [Troubleshooting](#troubleshooting)
- [FAQ](#faq)
- [Contributing](#contributing)
- [License](#license)

## What is UTCP?

UTCP is a protocol and architectural approach for exposing tools to software that needs to call them. A tool has metadata describing what it does, how it should be called, and which provider exposes it. A client can discover that metadata and invoke the tool without needing to implement provider-specific business logic at every call site.

The important idea is separation of concerns:

1. Providers expose capabilities.
2. The UTCP client discovers those capabilities.
3. A local repository stores tool metadata.
4. Search selects relevant tools.
5. The client dispatches calls to the appropriate transport.
6. The provider performs the operation.
7. Results are returned through one Go API.

This makes UTCP useful as infrastructure for AI agents, automation platforms, developer tools, integration services, and ordinary backend applications.

## Why go-utcp?

A typical integration layer accumulates one SDK per remote system. An application may need separate code for HTTP APIs, CLIs, gRPC services, GraphQL APIs, WebSockets, MCP servers, and local utilities. go-utcp provides a common abstraction above those transports.

Instead of writing application logic such as:

```text
if provider is HTTP -> use HTTP client
if provider is gRPC -> use gRPC client
if provider is MCP -> use MCP SDK
if provider is CLI -> start process
```

your application can work with:

```go
result, err := client.CallTool(ctx, "provider.tool", args)
```

The transport-specific behavior stays below the tool-calling API.

## Core concepts

### Provider

A provider is a configured source of one or more tools. It contains enough metadata for go-utcp to discover and invoke those tools.

### Tool

A tool is a callable capability. It has a name, description, input metadata, and provider association.

### Tool repository

The repository stores discovered tool metadata. The default implementation is in memory, while interfaces allow applications to provide custom storage or search implementations.

### Transport

A transport knows how to communicate with a provider. HTTP, gRPC, MCP, CLI, WebSocket, and other transports can implement the same higher-level tool invocation model.

### Search

Search selects tools from the local repository. Applications can use provider-scoped lookup or broader search strategies.

### CodeMode

CodeMode lets a constrained Go-like program compose multiple tool calls in one execution. This is useful when an agent needs loops, branching, transformation, or multi-step orchestration.

## Features

- Unified discovery API.
- Unified tool invocation API.
- Streaming tool invocation.
- Provider registration and deregistration.
- Provider-scoped tool names.
- In-memory tool repository.
- Pluggable repository and search abstractions.
- HTTP and OpenAPI support.
- CLI process providers.
- Server-Sent Events.
- Streamable HTTP.
- WebSocket.
- gRPC and gNMI support.
- GraphQL queries and subscriptions.
- TCP and UDP transports.
- WebRTC data-channel transport.
- MCP over supported MCP transports.
- Local text-template providers.
- Environment variable substitution.
- `.env` loading.
- Runtime variables.
- CodeMode tool composition.
- Streaming result handling.
- Context-aware calls.
- Standalone examples.
- Go-native APIs.

## Requirements

- Go 1.25 or newer.
- Network access for network-backed providers.
- Provider-specific runtime dependencies where applicable.

Check the module and examples for transport-specific requirements before deploying a particular provider.

## Installation

Install the library with:

```sh
go get github.com/universal-tool-calling-protocol/go-utcp@latest
```

Then import it from Go:

```go
import utcp "github.com/universal-tool-calling-protocol/go-utcp"
```

CodeMode is available under its plugin package:

```go
import "github.com/universal-tool-calling-protocol/go-utcp/src/plugins/codemode"
```

## Quick start

The smallest useful example can use a local text provider, so no external server is required.

Create `providers.json`:

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

Create a Go program:

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

Run it with:

```sh
go run .
```

The important detail is the qualified tool name:

```text
<provider>.<tool>
```

For the example above:

```text
greetings.hello
```

Provider qualification prevents ambiguous tool names when several providers expose tools with the same local name.

## Architecture

At a high level, go-utcp follows this flow:

```text
                  Provider configuration
                           |
                           v
                    Provider loader
                           |
                           v
                  Provider registration
                           |
                           v
                    Tool discovery
                           |
                           v
                    Tool repository
                           |
                 +---------+---------+
                 |                   |
                 v                   v
              Search              Lookup
                 |                   |
                 +---------+---------+
                           |
                           v
                      Tool call
                           |
                           v
                    Transport layer
                           |
                           v
                        Provider
```

The application normally interacts with the client rather than directly with individual transport implementations.

### Discovery phase

During provider registration, the client asks a provider to expose its tool metadata. The metadata is stored in the configured repository.

### Selection phase

An application or agent searches the repository to identify a tool. The search result gives the caller enough information to decide which tool to invoke.

### Invocation phase

The client resolves the qualified tool name, finds the associated provider and transport, validates the call path, and dispatches the invocation.

### Streaming phase

When a tool supports streaming, `CallToolStream` exposes a `StreamResult`. Consumers can incrementally read values until the stream reaches `io.EOF` or another error.

## Providers

Providers are the central configuration unit in go-utcp. A provider normally contains:

- A provider type.
- A unique provider name.
- Transport-specific configuration.
- Endpoint or command information where required.
- Optional headers or credentials.
- Tool discovery metadata.
- Provider-specific options.

Provider names should be stable, descriptive, and unique within a client configuration.

Good names include:

```text
billing
catalog
github
internal-search
weather
payments
```

Avoid names that expose credentials or unstable implementation details.

## Provider configuration

`ProvidersFilePath` can point at a JSON document. The loader accepts several root shapes.

### Array of providers

```json
[
  {
    "provider_type": "text",
    "name": "greetings",
    "templates": {
      "hello": "Hello, {{.name}}!"
    }
  }
]
```

### Single provider object

```json
{
  "provider_type": "text",
  "name": "greetings",
  "templates": {
    "hello": "Hello, {{.name}}!"
  }
}
```

### Providers object

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

Every provider must identify its `provider_type`.

### Built-in provider types

| Provider type | Typical purpose |
| --- | --- |
| `http` | HTTP/HTTPS APIs and OpenAPI-backed tools |
| `cli` | Local command-line programs |
| `sse` | Server-Sent Events |
| `http_stream` | Streamable HTTP |
| `websocket` | Bidirectional WebSocket communication |
| `grpc` | gRPC and gNMI services |
| `graphql` | GraphQL queries and subscriptions |
| `tcp` | Raw TCP services |
| `udp` | Raw UDP services |
| `webrtc` | WebRTC data channels |
| `mcp` | Model Context Protocol servers |
| `text` | Local Go text templates |

The exact fields vary by provider type. The `examples/` directory is the best place to inspect complete configurations for the current version.

## Variables and secrets

Provider configuration supports `$NAME` and `${NAME}` references.

Values are resolved from explicit client configuration, configured variable sources, and the process environment.

Example:

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

Provider configuration can then contain:

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

Do not commit production secrets into provider files.

For local development, `.env` files can be convenient. For production, prefer the secret-management mechanism provided by your deployment platform.

### Variable precedence

The intended lookup order is:

1. Explicit `UtcpClientConfig.Variables`.
2. Values loaded from configured variable sources such as `.env`.
3. Process environment variables.

This allows applications to override configuration without rewriting provider definitions.

## Tool discovery

Tool discovery happens when a provider is registered. The client obtains the provider's available tools and stores them in its repository.

A discovery result generally includes information such as:

- Qualified tool name.
- Human-readable description.
- Input schema or metadata.
- Provider association.
- Transport information.
- Provider-specific details.

Applications can use discovery to build dynamic agent tool menus, command palettes, workflow planners, or API integration layers.

## Searching tools

Use `SearchTools` to inspect the currently registered tool set.

To search all providers:

```go
tools, err := client.SearchTools("", 20)
if err != nil {
    return err
}
```

To restrict the search to a provider prefix:

```go
tools, err := client.SearchTools("github", 20)
if err != nil {
    return err
}
```

The search limit controls the maximum number of returned results supported by the search implementation.

### Search in an agent

A common agent loop looks like:

```text
User request
     |
     v
Tool search
     |
     v
Candidate tools
     |
     v
Tool selection
     |
     v
Tool invocation
     |
     v
Result interpretation
```

This architecture avoids loading every possible provider into every individual tool call while still allowing dynamic discovery.

## Calling tools

The main invocation API is:

```go
result, err := client.CallTool(ctx, "provider.tool", args)
```

Arguments are normally represented as a Go map:

```go
args := map[string]any{
    "query": "golang",
    "limit": 10,
}
```

Then:

```go
result, err := client.CallTool(ctx, "search.query", args)
```

Always pass the context associated with the operation so cancellation and deadlines can propagate to the transport when supported.

### Empty arguments

Tools that require no arguments can be called with an empty map:

```go
result, err := client.CallTool(ctx, "health.check", map[string]any{})
```

Whether an empty argument set is valid is determined by the tool's input contract.

### Result handling

The returned value is transport-independent from the caller's perspective. Applications should inspect or decode it according to the tool contract rather than assuming every transport returns the same concrete representation.

## Client API

| Method | Purpose |
| --- | --- |
| `RegisterToolProvider` | Discover and store tools from a provider. |
| `DeregisterToolProvider` | Remove a provider and its tools. |
| `SearchTools` | Search registered tools. |
| `CallTool` | Invoke a tool synchronously. |
| `CallToolStream` | Invoke a streaming tool. |
| `GetTransports` | Access registered transport implementations. |

The Go API may grow as UTCP evolves. For the authoritative signatures and types, use the package documentation on pkg.go.dev and the source tree.

## Streaming

Streaming is exposed through `CallToolStream`.

A basic consumer is:

```go
stream, err := client.CallToolStream(ctx, "events.watch", args)
if err != nil {
    return err
}
defer stream.Close()

for {
    item, err := stream.Next()
    if errors.Is(err, io.EOF) {
        break
    }
    if err != nil {
        return err
    }

    fmt.Println(item)
}
```

Always close a stream when the consumer stops early.

### Stream lifecycle

A typical lifecycle is:

```text
CallToolStream
      |
      v
  StreamResult
      |
      +--> Next()
      |
      +--> Next()
      |
      +--> Next()
      |
      v
   io.EOF
      |
      v
   Close()
```

Transport implementations may have different buffering and backpressure characteristics.

### Cancellation

Use a context with a deadline or cancellation signal when streaming may run for an unbounded period.

```go
ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
defer cancel()

stream, err := client.CallToolStream(ctx, "events.watch", args)
```

## Dynamic provider registration

Providers do not have to be static for the lifetime of an application. Applications can register providers at runtime and remove them later.

This is useful for:

- Multi-tenant applications.
- Plugin systems.
- Agent sessions.
- Temporary credentials.
- Ephemeral environments.
- User-selected integrations.
- Service discovery.

Registering a provider causes its tools to be discovered and made available through the client repository.

Deregistering a provider removes its tools from the active repository.

## Transport matrix

| Transport | Discovery | Calls | Streaming | Typical use |
| --- | --- | --- | --- | --- |
| HTTP | Yes | Yes | Depends on provider | REST-style services |
| CLI | Yes | Yes | Depends on command | Local tools |
| SSE | Yes | Yes | Yes | Event-oriented services |
| Streamable HTTP | Yes | Yes | Yes | HTTP streaming |
| WebSocket | Yes | Yes | Yes | Bidirectional services |
| gRPC | Yes | Yes | Yes | Typed service APIs |
| GraphQL | Yes | Yes | Yes | GraphQL APIs |
| TCP | Provider-dependent | Yes | Provider-dependent | Custom protocols |
| UDP | Provider-dependent | Yes | Provider-dependent | Datagram services |
| WebRTC | Provider-dependent | Yes | Yes | Peer/data-channel services |
| MCP | Yes | Yes | Yes, depending on server | MCP tool ecosystems |
| Text | Yes | Yes | No | Local deterministic helpers |

Capabilities depend on the provider and tool, not only the transport category.

## HTTP

HTTP providers are appropriate for ordinary HTTP and HTTPS services.

A typical configuration identifies the endpoint, HTTP method, provider name, and any required headers.

Example shape:

```json
{
  "provider_type": "http",
  "name": "catalog",
  "http_method": "POST",
  "url": "https://api.example.com/tools",
  "headers": {
    "Authorization": "Bearer ${API_TOKEN}"
  }
}
```

Use environment substitution for credentials rather than hard-coding tokens into source control.

HTTP deployments should also consider:

- TLS verification.
- Request timeouts.
- Authentication rotation.
- Proxy configuration.
- Server-side rate limits.
- Retries at the appropriate layer.

## OpenAPI

The HTTP integration can discover tools from OpenAPI-backed services.

OpenAPI is useful when an existing API already describes operations, parameters, request bodies, and responses through a machine-readable specification.

A practical integration flow is:

```text
OpenAPI document
      |
      v
HTTP provider
      |
      v
UTCP discovery
      |
      v
Tool metadata
      |
      v
Agent / application
```

When using OpenAPI discovery, keep the generated tool surface intentionally scoped. Large APIs can expose hundreds of operations, and applications may benefit from selecting only the operations required by a particular workflow.

## CLI

The CLI provider exposes local executable commands as tools.

CLI tools are useful for:

- Build systems.
- Developer assistants.
- Repository automation.
- Local infrastructure utilities.
- Deterministic command wrappers.

Treat CLI providers as privileged integrations. A tool capable of executing a local process can potentially modify files, access credentials, consume resources, or affect the host system.

Use least privilege and explicit command configuration.

## SSE

Server-Sent Events provide a server-to-client streaming mechanism over HTTP.

SSE is appropriate for services where the server produces an event stream and the client consumes events incrementally.

Applications should account for:

- Connection lifetime.
- Reconnection semantics.
- Cancellation.
- Event ordering.
- Partial failures.
- Server-side timeouts.

## Streamable HTTP

Streamable HTTP provides streaming behavior over HTTP without requiring a WebSocket connection.

It is useful when infrastructure is optimized for HTTP and the application still needs incremental results.

Use context cancellation to stop long-running streams and close resources promptly.

## WebSocket

WebSocket providers are useful when the client and server need a persistent bidirectional channel.

Common applications include:

- Interactive services.
- Real-time APIs.
- Event feeds.
- Agent runtimes.
- Low-latency integrations.

Production deployments should consider connection limits, idle timeouts, authentication renewal, and reconnect behavior.

## gRPC

The gRPC provider supports gRPC-style services and gNMI-related integrations exposed by the implementation.

gRPC is useful for internal service-to-service communication because it provides strongly typed service contracts and efficient binary transport.

When deploying gRPC tools, pay attention to:

- TLS configuration.
- Connection reuse.
- Deadlines.
- Streaming RPC lifecycle.
- Authentication.
- Service compatibility.

## GraphQL

GraphQL providers expose GraphQL operations as tools.

GraphQL can be particularly useful when callers need flexible selection of fields without maintaining a separate endpoint for every view.

GraphQL subscriptions can provide streaming behavior where supported by the configured transport.

Be deliberate about exposing mutation operations to agents because a GraphQL mutation may have significant side effects.

## TCP

The TCP transport is intended for services that communicate over raw TCP connections.

Because TCP itself does not define an application-level message format, the provider configuration and protocol implementation determine how requests and responses are framed.

Applications using raw TCP should define explicit framing and timeout behavior.

## UDP

UDP is appropriate for datagram-oriented services where connectionless delivery is part of the protocol design.

UDP does not provide the delivery guarantees of TCP. Applications should therefore understand packet loss, ordering, duplication, and message-size limitations before exposing UDP operations as tools.

## WebRTC

WebRTC support enables tool communication through data channels in environments where peer-to-peer communication is useful.

Typical considerations include:

- Signaling.
- NAT traversal.
- ICE configuration.
- Peer lifecycle.
- Authentication.
- Data-channel reliability settings.

Use WebRTC when its connectivity model provides a meaningful advantage over ordinary HTTP or WebSocket communication.

## MCP

The MCP provider allows go-utcp to integrate MCP-based tool servers into the same tool abstraction.

This is valuable when an application already has MCP servers but wants a broader provider and transport model around them.

The conceptual flow is:

```text
MCP server
    |
    v
MCP transport
    |
    v
UTCP provider
    |
    v
UTCP tool repository
    |
    v
Application / agent
```

The MCP integration can coexist with non-MCP providers. This means an application can expose HTTP, gRPC, GraphQL, CLI, and MCP tools through the same client.

### MCP registry metadata

When using MCP registries or registry-backed discovery, distinguish between registry metadata and the actual callable provider configuration. A registry can identify available MCP servers, while a provider configuration describes how the application connects to a selected server.

## Text providers

Text providers are useful for local deterministic tools and examples.

A simple provider can use Go text templates:

```json
{
  "provider_type": "text",
  "name": "greetings",
  "templates": {
    "hello": "Hello, {{.name}}!"
  }
}
```

Text providers are particularly useful for:

- Tests.
- Examples.
- Prototyping.
- Local transformations.
- Tool discovery experiments.

They are also a convenient way to understand the provider lifecycle without requiring a network service.

## CodeMode

CodeMode provides a controlled execution environment for composing multiple registered tool calls.

The motivation is simple: an agent sometimes needs more than one independent tool invocation. It may need to:

1. Search for resources.
2. Iterate over results.
3. Call another tool for each item.
4. Transform intermediate values.
5. Branch based on results.
6. Return one final value.

Doing every intermediate step through separate model round trips can be expensive. CodeMode moves some orchestration into a constrained program.

### Basic CodeMode example

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

The exact sandbox restrictions and supported language surface are implementation details and should be checked against the current CodeMode package.

### CodeMode helpers

CodeMode snippets can use helpers including:

- `codemode.CallTool`.
- `codemode.CallToolStream`.
- `codemode.SearchTools`.

The resulting `CodeModeResult` contains the produced value and captured output streams.

### Why CodeMode?

Without CodeMode, an agent may need to repeatedly request a tool, wait for a result, send the result back to the model, and request the next tool. CodeMode can reduce this orchestration overhead for deterministic multi-step logic.

### Tool safety in CodeMode

CodeMode should not be treated as unrestricted host execution. Tool access should remain bounded by the registered provider and the CodeMode execution policy.

Applications embedding CodeMode should carefully define:

- Which tools are available.
- Which providers are trusted.
- Execution timeouts.
- Resource limits.
- Filesystem access.
- Network access.
- Process access.
- Error handling.

## Security

go-utcp is an integration layer. It can connect an application to powerful capabilities, so security must be designed around the capabilities exposed through providers.

### Principle of least privilege

Expose only the tools an application actually needs.

A production agent rarely needs every tool from every provider.

Prefer:

```text
agent -> selected read-only tools
```

over:

```text
agent -> every provider -> every operation
```

### Credentials

Keep credentials outside committed provider configuration.

Prefer:

- Environment variables.
- Secret managers.
- Workload identity.
- Short-lived credentials.
- Per-provider credentials.

Avoid putting long-lived API keys in JSON files committed to source control.

### Tool permissions

Not every tool is equally safe.

A useful operational classification is:

| Category | Example | Typical policy |
| --- | --- | --- |
| Read-only | Search records | Usually automatic |
| Low-risk write | Create temporary resource | Review depending on context |
| Destructive | Delete data | Explicit approval |
| Privileged | Execute host command | Strong isolation |
| Credential-sensitive | Rotate secrets | Explicit authorization |

The protocol does not replace application authorization. Your application remains responsible for deciding which caller can invoke which provider.

### Network security

For remote providers:

- Prefer TLS.
- Validate certificates.
- Use authentication appropriate to the service.
- Set reasonable timeouts.
- Restrict egress where possible.
- Monitor unusual request patterns.

### Local execution

CLI and similar local providers can be highly privileged. Do not expose arbitrary command execution to untrusted callers.

Use fixed command definitions, argument validation, restricted environments, and OS-level isolation where appropriate.

## Error handling

All primary client operations return errors through idiomatic Go error values.

Always check them:

```go
result, err := client.CallTool(ctx, "catalog.search", args)
if err != nil {
    return fmt.Errorf("call catalog.search: %w", err)
}
```

Useful error boundaries include:

1. Configuration loading.
2. Provider registration.
3. Tool discovery.
4. Tool lookup.
5. Transport connection.
6. Remote execution.
7. Result decoding.
8. Stream consumption.

Wrap errors with operation context while preserving the original error using `%w`.

## Context cancellation

Go contexts should be propagated through every request path.

For bounded calls:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

result, err := client.CallTool(ctx, "catalog.search", args)
```

For user-driven cancellation:

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
```

A canceled context should cause the application to stop waiting for work that is no longer relevant.

## Testing

The repository contains tests covering core behavior and transport integrations.

Run the complete suite with:

```sh
go test ./...
```

For verbose output:

```sh
go test -v ./...
```

For race detection:

```sh
go test -race ./...
```

Run focused tests when iterating on one package:

```sh
go test ./path/to/package
```

### Testing with text providers

Text providers are useful for unit tests because they do not require external infrastructure.

A test can create a local provider, register it, search its tools, and call the tool.

This makes it possible to exercise the higher-level client flow without depending on network availability.

### Integration tests

Network-backed integrations should be tested with deterministic local servers where possible.

Tests should cover:

- Successful discovery.
- Successful invocation.
- Invalid configuration.
- Missing tools.
- Provider deregistration.
- Context cancellation.
- Stream completion.
- Stream failure.
- Authentication failures.
- Malformed provider responses.

## Examples

The [`examples`](examples/README.md) directory contains standalone examples for providers and transports.

A typical example can be run from its own directory:

```sh
cd examples/text_client
GOWORK=off go run -mod=mod .
```

The `GOWORK=off` setting is useful when an example is maintained as its own Go module and should use its own dependency graph.

### Example categories

Examples cover patterns such as:

- Basic client creation.
- Provider configuration.
- Text tools.
- HTTP providers.
- Streaming.
- MCP integrations.
- CodeMode.
- Transport-specific clients.

Always inspect the example's local README or source before running a network example because some examples expect a local service to be running.

## Development

Clone the repository:

```sh
git clone https://github.com/universal-tool-calling-protocol/go-utcp.git
cd go-utcp
```

Run tests:

```sh
go test ./...
```

Run formatting:

```sh
gofmt -w .
```

For changed Go files, format only the relevant paths if you prefer a smaller diff.

### Go modules

Use the repository's declared Go version and module dependencies. Avoid introducing unnecessary dependencies for functionality that can be implemented with the standard library or existing project abstractions.

### Onboarding

New contributors can follow [`onboarding.md`](onboarding.md).

## Project layout

The repository is organized around the client, provider, transport, repository, plugin, and example layers.

A conceptual layout is:

```text
go-utcp/
├── client / core package
├── providers
├── transports
├── repository abstractions
├── plugins/
│   └── codemode/
├── examples/
├── tests
├── onboarding.md
└── README.md
```

The exact source layout can change as the project evolves. Prefer package names and Go documentation over relying on private implementation details.

## Performance

go-utcp is intended to provide a common abstraction without forcing every caller to implement its own integration stack.

Performance depends heavily on the provider and transport.

For local operations, overhead can be dominated by provider discovery, serialization, process startup, or the selected repository implementation.

For network operations, network latency, server execution time, TLS, connection setup, serialization, and remote rate limits generally dominate.

### Performance guidelines

- Reuse a client where appropriate.
- Avoid repeatedly discovering the same provider unnecessarily.
- Keep provider metadata scoped to what the application needs.
- Use streaming when incremental results are valuable.
- Apply context deadlines.
- Avoid unnecessary serialization between agent layers.
- Measure the real provider path rather than optimizing only local code.

### Benchmarks

When adding performance-sensitive functionality, benchmark representative workloads with Go's benchmark tooling:

```sh
go test -bench=. ./...
```

Benchmarks should document their workload and avoid making claims based on synthetic microbenchmarks alone.

## Observability

Applications embedding go-utcp should instrument important boundaries.

Useful metrics include:

- Tool calls by provider.
- Tool calls by tool name.
- Success and error counts.
- Latency.
- Stream duration.
- Stream item count.
- Provider discovery time.
- Discovery failures.
- Cancellation counts.

Avoid logging secrets or sensitive tool arguments.

For structured logging, record identifiers such as provider and tool names while redacting credentials and sensitive payloads.

## Reliability

Reliable tool execution requires more than transport retries.

Consider:

- Idempotency.
- Deadlines.
- Retryable versus non-retryable errors.
- Duplicate side effects.
- Connection reuse.
- Provider health.
- Circuit breaking at the application layer.

Do not blindly retry destructive operations. A timeout after a remote write does not necessarily mean the remote write did not happen.

## Agent integration

go-utcp can act as the tool layer underneath an agent runtime.

A common architecture is:

```text
                   User
                    |
                    v
                 Agent
                    |
              Tool selection
                    |
                    v
               go-utcp
                    |
       +------------+-------------+
       |            |             |
      HTTP         MCP          gRPC
       |            |             |
       v            v             v
    Service      Server        Service
```

The agent can search available tools rather than hard-coding every integration.

### Tool selection loop

A robust agent can use this loop:

```text
1. Receive task
2. Search tools
3. Inspect descriptions and schemas
4. Select minimum required capability
5. Invoke tool
6. Validate result
7. Continue or finish
```

This approach keeps the tool surface dynamic while allowing application-level policy to remain in control.

## Multi-provider applications

One client can expose tools from several providers.

For example:

```text
github.search
postgres.query
weather.current
billing.invoice
filesystem.read
```

The agent does not need to know that one tool uses HTTPS, another uses gRPC, and another uses a local command.

Provider qualification provides a stable namespace boundary.

## Tool naming

Use provider names that communicate ownership or domain.

Examples:

```text
github.search_repositories
github.create_issue
billing.get_invoice
billing.create_invoice
catalog.search
catalog.get_product
```

Avoid unnecessarily generic provider names such as `api`, `service`, or `tools` when multiple integrations exist.

Stable names make logs, metrics, agent prompts, and authorization policies easier to reason about.

## Configuration management

For small applications, a single `providers.json` file can be sufficient.

For larger deployments, consider generating or assembling provider configuration from:

- Environment-specific templates.
- Secret managers.
- Deployment configuration.
- Service discovery.
- Tenant configuration.

Keep the distinction between configuration and credentials clear.

A provider definition should describe how a capability is reached. A secret manager should provide sensitive values needed to authenticate to it.

## Production checklist

Before deploying a go-utcp-based integration, verify:

- [ ] Provider names are unique.
- [ ] Credentials are not committed.
- [ ] TLS is enabled for sensitive remote traffic.
- [ ] Context deadlines are configured.
- [ ] Tool permissions are intentionally scoped.
- [ ] Destructive tools require appropriate authorization.
- [ ] Local command providers are sandboxed where necessary.
- [ ] Streaming resources are closed.
- [ ] Errors are logged with useful context.
- [ ] Sensitive arguments are redacted from logs.
- [ ] Provider failures are observable.
- [ ] Retry behavior is appropriate for each operation.
- [ ] Integration tests cover failure paths.
- [ ] Dependency versions are reviewed before release.

## Troubleshooting

### No tools are returned

Check that:

1. The provider file exists.
2. The JSON is valid.
3. The provider has a valid `provider_type`.
4. The provider name is present.
5. The provider can be reached when discovery requires a network service.
6. Discovery does not require credentials that are missing.

Then run a minimal `SearchTools` call and inspect the returned error.

### Tool not found

If:

```go
client.CallTool(ctx, "foo.bar", args)
```

returns a missing-tool error, first call:

```go
client.SearchTools("", 100)
```

and verify the exact qualified name.

Remember that tools are normally addressed as:

```text
provider.tool
```

### Environment variable is not resolved

Check:

- Variable spelling.
- `$NAME` versus `${NAME}` syntax.
- `Variables` configuration.
- `.env` path.
- Process environment.
- Configuration load order.

Do not print secret values while debugging.

### Network call times out

Check:

- Provider endpoint.
- DNS.
- TLS.
- Proxy configuration.
- Server health.
- Context deadline.
- Network policy.

A timeout may indicate either client-side cancellation or a remote service that is not responding.

### Stream never ends

Some tools intentionally expose long-lived streams. If the application expects a bounded operation, use a context deadline or cancellation policy.

Always close streams when abandoning consumption.

### CLI provider fails

Check:

- Executable availability.
- PATH.
- File permissions.
- Working directory.
- Environment variables.
- Argument encoding.
- Process exit status.

For production workloads, prefer narrowly defined commands over arbitrary shell execution.

## FAQ

### Is go-utcp an MCP replacement?

No. go-utcp can integrate MCP providers while also supporting many other provider types and transports. It is intended as a broader tool-calling abstraction.

### Can I use only HTTP?

Yes. You can configure only the providers your application needs.

### Can multiple providers expose the same tool name?

Yes. Provider-qualified names prevent collisions.

### Can providers be added at runtime?

Yes. The client exposes provider registration and deregistration APIs.

### Does every tool support streaming?

No. Streaming depends on the tool and its transport/provider implementation.

### Can I use a custom repository?

The architecture provides repository abstractions so applications can supply custom storage/search behavior where supported by the current API.

### Can I use go-utcp without an AI agent?

Yes. UTCP is useful for ordinary applications and automation systems as well. An agent is only one possible consumer.

### Is CodeMode required?

No. CodeMode is an optional plugin for applications that need programmatic multi-tool composition.

### Should I expose every provider to an agent?

Usually not. Restrict the active tool set to the minimum capabilities required for the current task.

## Design principles

The project follows several practical principles.

### One abstraction, many transports

Application code should not have to understand every transport implementation.

### Explicit capability discovery

Tools should be discoverable and describable before they are invoked.

### Provider isolation

Provider boundaries should remain visible through names and configuration.

### Go-native integration

The library should feel natural in Go applications, using contexts, errors, interfaces, maps, and standard tooling.

### Extensibility

New providers, repositories, transports, and plugins should be possible without forcing every application to change its architecture.

### Operational safety

The library should make it possible for host applications to implement authorization, observability, cancellation, and resource controls.

## Extending go-utcp

When adding a new transport or provider, consider the complete lifecycle rather than implementing only the happy-path call.

A useful checklist is:

1. Configuration structure.
2. Validation.
3. Provider registration.
4. Tool discovery.
5. Tool invocation.
6. Streaming if supported.
7. Context cancellation.
8. Resource cleanup.
9. Error propagation.
10. Tests.
11. Example.
12. Documentation.

### New transport checklist

A transport implementation should define clearly:

- How connections are established.
- How tools are discovered.
- How requests are encoded.
- How responses are decoded.
- How errors are represented.
- Whether streaming is supported.
- How cancellation works.
- How resources are closed.

### New provider checklist

A provider should document:

- Required configuration.
- Optional configuration.
- Authentication.
- Discovery behavior.
- Invocation behavior.
- Streaming behavior.
- Security considerations.
- Example usage.

## Compatibility

Because go-utcp integrates external services and protocols, compatibility depends on both the library version and the remote provider implementation.

When upgrading:

1. Read the changelog or release notes when available.
2. Run the full test suite.
3. Run transport integration tests.
4. Verify provider configuration.
5. Check CodeMode compatibility if used.
6. Test streaming providers.
7. Validate authentication behavior.

Pin versions in production according to your dependency-management policy rather than assuming every provider is immutable.

## Release hygiene

Before publishing a release, maintainers should consider:

- `go test ./...`.
- `go vet ./...` where applicable.
- Formatting.
- Dependency review.
- Example compilation.
- Documentation accuracy.
- Transport compatibility.
- Security review for new providers.
- Changelog/release notes.

## Documentation sources

Useful project resources include:

- Repository source.
- [`examples/`](examples/README.md).
- [`onboarding.md`](onboarding.md).
- Go package documentation.
- UTCP protocol documentation.
- Provider-specific protocol specifications.

The repository source is authoritative for behavior implemented by the current version.

## Contributing

Contributions are welcome.

Before making a larger change, inspect existing interfaces and provider patterns so that new functionality fits the architecture instead of creating a parallel abstraction.

For a change, a good workflow is:

```text
Understand
   |
   v
Implement
   |
   v
Format
   |
   v
Test
   |
   v
Document
   |
   v
Review
```

### Pull requests

A useful pull request should explain:

- What changed.
- Why it changed.
- Which packages are affected.
- How it was tested.
- Whether configuration changes are required.
- Whether compatibility is affected.

Keep unrelated refactors out of focused changes where practical.

### Documentation contributions

Documentation should prefer executable examples and concrete configuration over vague descriptions.

When adding a new provider, include a minimal example whenever possible.

## Repository examples

The examples directory is intentionally part of the documentation surface. When behavior changes, examples should be updated if their assumptions no longer match the implementation.

A good example should:

- Be small.
- Be runnable.
- Show realistic configuration.
- Avoid unnecessary dependencies.
- Explain external prerequisites.
- Demonstrate correct error handling.

## Frequently useful commands

Clone:

```sh
git clone https://github.com/universal-tool-calling-protocol/go-utcp.git
```

Enter:

```sh
cd go-utcp
```

Test:

```sh
go test ./...
```

Race test:

```sh
go test -race ./...
```

Vet:

```sh
go vet ./...
```

Format:

```sh
gofmt -w .
```

Benchmark:

```sh
go test -bench=. ./...
```

Run a standalone example:

```sh
cd examples/text_client
GOWORK=off go run -mod=mod .
```

## Minimal reference application

A compact application generally needs only four steps:

```go
ctx := context.Background()

client, err := utcp.NewUTCPClient(ctx, config, nil, nil)
if err != nil {
    return err
}

tools, err := client.SearchTools("", 10)
if err != nil {
    return err
}

_ = tools

result, err := client.CallTool(ctx, "provider.tool", map[string]any{})
if err != nil {
    return err
}

_ = result
```

This is the core mental model of go-utcp: configure providers, discover tools, select a tool, and call it.

## Mental model for production systems

A production deployment can be thought of as five layers:

```text
Policy
  |
  v
Agent / Application
  |
  v
UTCP client
  |
  v
Provider + transport
  |
  v
External capability
```

Policy determines what is allowed.

The application determines what it wants to accomplish.

The UTCP client provides the common tool interface.

The provider and transport determine how the operation is delivered.

The external capability performs the actual work.

Keeping these responsibilities separate makes the system easier to test, secure, and evolve.

## Common anti-patterns

### Exposing every tool

Large unrestricted tool sets make authorization and agent selection harder.

Prefer task-specific tool scopes.

### Hard-coding credentials

Never place production credentials directly into provider JSON committed to source control.

### Ignoring contexts

Long-running calls should have deadlines or cancellation paths.

### Blind retries

Retries can duplicate side effects. Understand idempotency first.

### Treating CodeMode as unrestricted execution

CodeMode should have explicit tool and execution boundaries.

### Logging complete arguments

Tool arguments can contain secrets or personal data. Redact sensitive fields.

### Depending on transport internals

Application code should prefer the common client API unless it genuinely needs transport-specific functionality.

## Scaling considerations

For applications with many providers or thousands of tools, discovery and search become operational concerns.

Consider:

- Scoped discovery.
- Provider grouping.
- Search indexing.
- Custom repositories.
- Tool metadata caching.
- Tenant isolation.
- Tool lifecycle management.
- Explicit active-tool sets.

The default in-memory repository is convenient for many applications. Larger systems may benefit from a persistent or specialized search implementation.

## Multi-tenant systems

In a multi-tenant application, provider configuration and credentials should generally be scoped to the tenant rather than shared globally.

A safe conceptual model is:

```text
Tenant A -> providers A -> tools A
Tenant B -> providers B -> tools B
Tenant C -> providers C -> tools C
```

Do not allow one tenant's provider credentials or tools to become visible to another tenant through a shared mutable registry without explicit isolation.

## Long-running agents

Long-running agents should treat provider configuration as mutable infrastructure.

Providers can disappear, credentials can expire, and remote services can change availability.

Useful practices include:

- Periodic health checks outside the tool-call path.
- Credential rotation.
- Explicit provider lifecycle events.
- Time-bounded calls.
- Observability.
- Graceful stream shutdown.

## Graceful shutdown

Applications should cancel outstanding contexts and close long-lived streams during shutdown.

A typical service lifecycle is:

```text
Start
  |
  v
Load configuration
  |
  v
Register providers
  |
  v
Serve requests
  |
  v
Cancel active work
  |
  v
Close streams/providers
  |
  v
Exit
```

The exact shutdown API depends on the provider and transport implementation, so applications should follow the lifecycle documented by the concrete provider they use.

## Security review questions

Before exposing a tool to an autonomous caller, ask:

1. What can this tool change?
2. What credentials does it use?
3. What data can it read?
4. Can the operation be reversed?
5. Is it idempotent?
6. What is the blast radius of misuse?
7. Does it need human approval?
8. Can arguments be constrained?
9. Can output contain secrets?
10. What happens if the provider is compromised?

These questions are especially important for filesystem, shell, database, deployment, billing, and administrative tools.

## Summary

go-utcp provides a common Go interface for heterogeneous tool ecosystems.

The core workflow is intentionally simple:

```text
Configure providers
       |
       v
Discover tools
       |
       v
Search tools
       |
       v
Call tools
       |
       v
Consume results
```

Around that core, the project provides multiple transports, streaming, runtime provider lifecycle, variable substitution, MCP integration, OpenAPI discovery, and CodeMode composition.

For a small application, start with the quick-start example and a text provider. For a production integration, add the transport-specific configuration, authorization, observability, timeouts, testing, and security controls required by your environment.

## Project links

- Repository: https://github.com/universal-tool-calling-protocol/go-utcp
- UTCP organization: https://github.com/universal-tool-calling-protocol
- Go package documentation: https://pkg.go.dev/github.com/universal-tool-calling-protocol/go-utcp
- Examples: [`examples/`](examples/README.md)
- Contributor onboarding: [`onboarding.md`](onboarding.md)

## License

This project is licensed under the [Mozilla Public License 2.0](LICENSE).

Copyright and licensing information for individual files or dependencies may differ; consult their respective notices where applicable.
