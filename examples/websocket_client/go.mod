module example-websocket-client

go 1.23.0

toolchain go1.24.3

require (
    github.com/universal-tool-calling-protocol/go-utcp v0.0.0
    github.com/gorilla/websocket v1.5.3
)

replace github.com/universal-tool-calling-protocol/go-utcp => ../../
