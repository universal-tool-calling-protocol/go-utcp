module main

go 1.25.0

require (
	github.com/gorilla/websocket v1.5.3
	github.com/universal-tool-calling-protocol/go-utcp v0.0.0
)

require (
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
)

replace github.com/universal-tool-calling-protocol/go-utcp => ../..
