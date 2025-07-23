package udp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/concepts"

	"time"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers"
)

// UDPTransport implements the ClientTransport interface over UDP.
type UDPTransport struct {
	logger func(format string, args ...interface{})
}

// NewUDPTransport constructs a UDPTransport with optional logging.
func NewUDPTransport(logger func(format string, args ...interface{})) *UDPTransport {
	if logger == nil {
		logger = func(string, ...interface{}) {}
	}
	return &UDPTransport{logger: logger}
}

// writeAndRead sends a packet and waits for a response within timeout.
func (t *UDPTransport) writeAndRead(ctx context.Context, addr string, timeout time.Duration, payload []byte) ([]byte, error) {
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}
	if _, err := conn.Write(payload); err != nil {
		return nil, err
	}
	buf := make([]byte, 65535)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// RegisterToolProvider discovers tools by sending a DISCOVER message to the server.
func (t *UDPTransport) RegisterToolProvider(ctx context.Context, prov Provider) ([]Tool, error) {
	p, ok := prov.(*UDPProvider)
	if !ok {
		return nil, errors.New("UDPTransport can only be used with UDPProvider")
	}
	addr := fmt.Sprintf("%s:%d", p.Host, p.Port)
	timeout := time.Duration(p.Timeout) * time.Millisecond
	resp, err := t.writeAndRead(ctx, addr, timeout, []byte("DISCOVER"))
	if err != nil {
		return nil, err
	}
	var manual UtcpManual
	if err := json.Unmarshal(resp, &manual); err != nil {
		return nil, err
	}
	return manual.Tools, nil
}

// DeregisterToolProvider is a no-op for UDPTransport.
func (t *UDPTransport) DeregisterToolProvider(ctx context.Context, prov Provider) error {
	if _, ok := prov.(*UDPProvider); !ok {
		return errors.New("UDPTransport can only be used with UDPProvider")
	}
	return nil
}

// CallTool sends a JSON request with tool name and arguments and waits for the response.
func (t *UDPTransport) CallTool(ctx context.Context, toolName string, args map[string]any, prov Provider, l *string) (any, error) {
	p, ok := prov.(*UDPProvider)
	if !ok {
		return nil, errors.New("UDPTransport can only be used with UDPProvider")
	}
	addr := fmt.Sprintf("%s:%d", p.Host, p.Port)
	timeout := time.Duration(p.Timeout) * time.Millisecond
	payload, err := json.Marshal(map[string]any{"tool": toolName, "args": args})
	if err != nil {
		return nil, err
	}
	resp, err := t.writeAndRead(ctx, addr, timeout, payload)
	if err != nil {
		return nil, err
	}
	var result any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return result, nil
}
