# Onboarding

Welcome to **go-utcp**, a Go implementation of the Universal Tool Calling Protocol (UTCP). This guide helps new contributors set up a development environment, run tests, and explore examples.

## Prerequisites
- [Go](https://go.dev/dl/) **1.23** or later
- `git`

## Clone and Build
```sh
git clone https://github.com/universal-tool-calling-protocol/go-utcp.git
cd go-utcp
```

Use the standard Go tooling to format and compile the project:
```sh
go fmt ./...
go build ./...
```

## Running Tests
Execute the full test suite to ensure your changes do not break existing functionality:
```sh
go test ./...
```

## Running Examples
Each directory under [`examples/`](examples) is a standalone module demonstrating various UTCP transports. When running an example, disable the workspace so Go uses the module's own `go.mod`:
```sh
GOWORK=off go run ./examples/cli_transport
```

## Contributing
1. Create a new branch and make your changes.
2. Run `go fmt` and `go test` before committing.
3. Submit a pull request describing your changes.

Welcome aboard and happy hacking!
