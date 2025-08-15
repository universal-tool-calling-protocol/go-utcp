# Examples

This directory contains standalone Go modules that demonstrate how to use the UTCP client and transports.

## Running an example

From the repository root, disable the workspace so each example uses its own `go.mod`:

```sh
GOWORK=off go run ./examples/cli_transport
```

Replace `cli_transport` with the directory of the example you want to run.

## Client examples

- `cli_client`: Connects to tools exposed over the command line.
- `graphql_client`: Calls tools provided via a GraphQL service.
- `grpc_client`: Uses the gRPC transport.
- `http_client`: Interacts with an HTTP provider.
- `mcp_client`: Talks to a provider using the MCP transport.
- `mcp_http_client`: Demonstrates the MCP HTTP transport.
- `sse_client`: Streams results with Server-Sent Events.
- `streamable_client`: Uses the streaming HTTP transport.
- `tcp_client`: Communicates over a raw TCP connection.
- `udp_client`: Sends requests over UDP.
- `websocket_client`: Uses a WebSocket connection.
- `webrtc_client`: Connects via WebRTC.
- `text_client`: Renders text templates locally.

## Transport examples

- `cli_transport`: Exposes tools via the command line.
- `graphql_transport`: Serves tools through a GraphQL endpoint.
- `grpc_transport`: Implements a gRPC provider.
- `http_transport`: Serves tools over HTTP.
- `mcp_transport`: Bridges UTCP to the MCP protocol.
- `mcp_http_transport`: Bridges UTCP to MCP over HTTP.
- `sse_transport`: Sends results using Server-Sent Events.
- `streamable_transport`: Demonstrates the streaming HTTP transport.
- `tcp_transport`: Provides tools over TCP.
- `udp_transport`: Provides tools over UDP.
- `websocket_transport`: Hosts tools on a WebSocket server.
- `webrtc_transport`: Serves tools using WebRTC.
- `text_transport`: Provides tools based on text templates.

