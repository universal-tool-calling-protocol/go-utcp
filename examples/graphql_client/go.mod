module example-graphql-client

go 1.23.0

toolchain go1.24.3

require github.com/universal-tool-calling-protocol/go-utcp v0.0.0

require (
	github.com/graphql-go/graphql v0.8.1 // indirect
	github.com/graphql-go/handler v0.2.4 // indirect
)

replace github.com/universal-tool-calling-protocol/go-utcp => ../../
