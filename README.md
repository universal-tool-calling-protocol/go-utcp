# UTCP

Universal Tool Calling Protocol (UTCP) reference implementation.

## Examples

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

The sample `http_provider.json` demonstrates how to specify an HTTP provider with
API key authentication.
