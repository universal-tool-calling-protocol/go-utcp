package tcp

import (
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

// TCPProvider represents a raw TCP socket.
type TCPProvider struct {
	BaseProvider
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Timeout int    `json:"timeout"` // ms, default 30000
	// auth always nil
}
