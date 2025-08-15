# Cursor Bridge Client Example

CLI client for the Cursor UTCP Bridge server.

## Quick Start

```bash
go build -o cursor-utcp

# List tools
./cursor-utcp -cmd list

# Call a tool
./cursor-utcp -cmd call -tool cli.echo -input '{"message": "Hello"}'

# Or use the shell wrapper
./cursor-utcp.sh call cli.echo message="Hello"
```

## Commands

- `list` - List all tools
- `search -query <term>` - Search tools
- `info -tool <name>` - Tool details
- `call -tool <name> -input <json>` - Execute tool
- `health` - Check server status
- `refresh` - Refresh tool cache

## Environment

- `CURSOR_UTCP_URL` - Server URL (default: http://localhost:8080)

## Shell Wrapper

```bash
./cursor-utcp.sh help              # Show usage
./cursor-utcp.sh list               # List tools
./cursor-utcp.sh call <tool> <args> # Call tool
```

## Testing

Run tests:
```bash
make test           # Run unit tests
make test-coverage  # Generate coverage report
make test-bench     # Run benchmarks
```
