package tcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/manual"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/tools"
)

// TCPClientTransport implements ClientTransport over raw TCP sockets.
type TCPClientTransport struct {
	logger func(format string, args ...interface{})
}

// NewTCPClientTransport creates a new instance with an optional logger.
func NewTCPClientTransport(logger func(format string, args ...interface{})) *TCPClientTransport {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}
	return &TCPClientTransport{logger: logger}
}

func (t *TCPClientTransport) dial(ctx context.Context, prov *TCPProvider) (net.Conn, error) {
	timeout := time.Duration(prov.Timeout)
	if timeout == 0 {
		timeout = 30000
	}
	d := net.Dialer{Timeout: timeout * time.Millisecond}
	return d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", prov.Host, prov.Port))
}

// RegisterToolProvider connects to the TCP provider and retrieves its manual.
func (t *TCPClientTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	tcpProv, ok := prov.(*TCPProvider)
	if !ok {
		return nil, errors.New("TCPClientTransport can only be used with TCPProvider")
	}
	conn, err := t.dial(ctx, tcpProv)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Request manual
	req := map[string]string{"action": "list"}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, err
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&resp); err != nil {
		return nil, err
	}
	manual := NewUtcpManualFromMap(resp)
	return manual.Tools, nil
}

// DeregisterToolProvider is a no-op for TCP transport.
func (t *TCPClientTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	if _, ok := prov.(*TCPProvider); !ok {
		return errors.New("TCPClientTransport can only be used with TCPProvider")
	}
	return nil
}

// CallTool connects to the provider and sends a tool invocation request.
func (t *TCPClientTransport) CallTool(ctx context.Context, toolName string, args map[string]any, prov Provider, requestID *string) (any, error) {
	tcpProv, ok := prov.(*TCPProvider)
	if !ok {
		return nil, errors.New("TCPClientTransport can only be used with TCPProvider")
	}

	// Open the TCP connection using the provider's connection info
	conn, err := t.dial(ctx, tcpProv)
	if err != nil {
		return nil, fmt.Errorf("tcp_transport: dial error: %w", err)
	}
	defer conn.Close()

	// Send the request payload
	req := map[string]any{"tool": toolName, "args": args}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, fmt.Errorf("tcp_transport: encode error: %w", err)
	}

	// Read the response
	dec := json.NewDecoder(bufio.NewReader(conn))
	var result any
	if err := dec.Decode(&result); err != nil {
		// If the peer just closed the socket without a payload, treat it as no result
		if errors.Is(err, io.EOF) {
			t.logger("tcp_transport: EOF reading result for tool %q, returning nil", toolName)
			return nil, nil
		}
		return nil, fmt.Errorf("tcp_transport: decode error: %w", err)
	}

	return result, nil
}

// Close cleans up resources (no-op).
func (t *TCPClientTransport) Close() error { return nil }
