# Cursor Bridge Transport Example

UTCP bridge server that aggregates tools from multiple providers into a unified API.

## Quick Start

1. Configure providers: Edit `provider-*.json` files
2. Run: `go run main.go`
3. Server starts on port 8080 (or set PORT env var)

## Usage

```bash
# List tools
curl http://localhost:8080/api/v1/tools

# Call a tool (format: provider.toolname)
curl -X POST http://localhost:8080/api/v1/tools/call \
  -H "Content-Type: application/json" \
  -d '{"tool": "cli.echo", "params": {"message": "Hello"}}'
```

## Configuration

Place provider JSON files in current directory. Example:
```json
{
  "name": "http-provider",
  "type": "http",
  "url": "http://localhost:8000"
}
```

See [architecture.md](architecture.md) for detailed system design.

## Testing

Comprehensive test suite included:
```bash
make test-all        # Run all tests
make test-coverage   # Generate coverage report
```

See [TESTING.md](TESTING.md) for testing guide.
