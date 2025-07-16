# UTCP

Universal Tool Calling Protocol (UTCP) reference implementation.

UTCP is an attempt to standardise how a client can discover and call
"tools" (or APIs) no matter what transport they live behind.  Each
provider describes a transport (HTTP, CLI, GraphQL and so on) and a set
of tools exposed by that transport.  This repository contains a small
Go client that can load provider definitions, register them at runtime
and then invoke the discovered tools.

Only a handful of transports are sketched out and several components
are intentionally left as stubs.  The goal of the code is to
demonstrate the basic flow rather than provide a production ready
implementation.

## Getting Started

To experiment with the client you can load one of the example provider
configurations and register it with a new `UtcpClient` instance.

Example provider configurations are located in the [examples](./examples/) directory.
These files can be loaded by the client using `UtcpClientConfig`:

```go
cfg := &UTCP.UtcpClientConfig{
    ProvidersFilePath: "examples/http_provider.json",
}
client, err := UTCP.NewUtcpClient(context.Background(), cfg, nil, nil)
if err != nil {
    log.Fatal(err)
}
```

The sample `http_provider.json` demonstrates how to specify an HTTP provider
with API key authentication.  Provider configuration files are JSON arrays so
multiple providers can be loaded at once.

Run `go run ./examples` to execute a small program that loads this provider,
registers it with the client and calls the first discovered tool.

## Provider Types

The client supports a number of provider types, each corresponding to a
different transport.  Only a subset is implemented but the JSON
configuration allows for the following kinds of providers:

- `http` and `http_stream` for REST style APIs
- `sse` for Server Sent Events streams
- `cli` for command line tools
- `text` to load tools from text files
- `graphql`, `grpc`, `websocket`, `tcp`, `udp`, `webrtc` and `mcp` are
  defined but largely unimplemented in this repository.
