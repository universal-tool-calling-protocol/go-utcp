package udp

import (
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

// UDPProvider represents a UDP socket.
type UDPProvider struct {
	BaseProvider
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Timeout int    `json:"timeout"`
	// auth always nil
}
