package transport

import (
	"context"
	"errors"
	"fmt"
	"server"
)

// SSEClientTransport implements Server-Sent Events over HTTP for UTCP tools.
type SSEClientTransport struct {
	// Add any fields you need, e.g., HTTP client, logger, etc.
	logger func(format string, args ...interface{})
}

// NewSSETransport constructs a new SSEClientTransport.
func NewSSETransport(logger func(format string, args ...interface{})) *SSEClientTransport {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}
	return &SSEClientTransport{logger: logger}
}

// RegisterToolProvider registers an SSE-based provider.
func (t *SSEClientTransport) RegisterToolProvider(ctx context.Context, prov server.Provider) ([]server.Tool, error) {
	sseProv, ok := prov.(*server.SSEProvider)
	if !ok {
		return nil, errors.New("SSETransport can only be used with SSEProvider")
	}
	// TODO: discover tools via SSE handshake or metadata
	return nil, fmt.Errorf("RegisterToolProvider not implemented for SSEProvider '%s'", sseProv.Name)
}

// DeregisterToolProvider cleans up any resources (no-op for SSE).
func (t *SSEClientTransport) DeregisterToolProvider(ctx context.Context, prov server.Provider) error {
	// TODO: close any open streams if necessary
	return nil
}

// CallTool invokes a named tool over SSE.
func (t *SSEClientTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}, prov server.Provider, l *string) (interface{}, error) {
	// TODO: connect to SSE endpoint, send request, and stream responses
	return nil, errors.New("CallTool not implemented for SSETransport")
}

// StreamableHTTPClientTransport implements HTTP with streaming support.
type StreamableHTTPClientTransport struct {
	// embed or reuse HttpClientTransport fields if desired
	logger func(format string, args ...interface{})
}

// NewStreamableHTTPTransport constructs a new StreamableHTTPClientTransport.
func NewStreamableHTTPTransport(logger func(format string, args ...interface{})) *StreamableHTTPClientTransport {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}
	return &StreamableHTTPClientTransport{logger: logger}
}

// RegisterToolProvider registers an HTTP streaming provider.
func (t *StreamableHTTPClientTransport) RegisterToolProvider(ctx context.Context, prov server.Provider) ([]server.Tool, error) {
	hp, ok := prov.(*server.HttpProvider)
	if !ok {
		return nil, errors.New("StreamableHTTPTransport can only be used with HttpProvider")
	}
	// TODO: similar to HTTP transport, but marking streaming-capable tools
	return nil, fmt.Errorf("RegisterToolProvider not implemented for streaming HTTP provider '%s'", hp.Name)
}

// DeregisterToolProvider cleans up streaming HTTP resources.
func (t *StreamableHTTPClientTransport) DeregisterToolProvider(ctx context.Context, prov server.Provider) error {
	return nil
}

// CallTool invokes a tool and returns a stream (io.Reader) or aggregated result.
func (t *StreamableHTTPClientTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}, prov server.Provider, l *string) (interface{}, error) {
	// TODO: send HTTP request and return io.Reader for streaming body
	return nil, errors.New("CallTool not implemented for StreamableHTTPTransport")
}

// MCPClientTransport implements a custom MCP protocol transport.
// Replace with actual protocol details.
type MCPClientTransport struct {
	// Add protocol-specific fields here
	logger func(format string, args ...interface{})
}

// NewMCPTransport constructs a new MCPClientTransport.
func NewMCPTransport(logger func(format string, args ...interface{})) *MCPClientTransport {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}
	return &MCPClientTransport{logger: logger}
}

func (t *MCPClientTransport) RegisterToolProvider(ctx context.Context, prov server.Provider) ([]server.Tool, error) {
	mcpProv, ok := prov.(*server.MCPProvider)
	if !ok {
		return nil, errors.New("MCPTransport can only be used with MCPProvider")
	}
	// TODO: implement discovery over MCP
	return nil, fmt.Errorf("RegisterToolProvider not implemented for MCPProvider '%s'", mcpProv.Name)
}

func (t *MCPClientTransport) DeregisterToolProvider(ctx context.Context, prov server.Provider) error {
	return nil
}

func (t *MCPClientTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}, prov server.Provider, l *string) (interface{}, error) {
	// TODO: send MCP-formatted request and parse response
	return nil, errors.New("CallTool not implemented for MCPTransport")
}

// TextClientTransport is a simple in-memory/text-based transport.
type TextClientTransport struct {
	prefix string
}

// NewTextTransport constructs a TextClientTransport.
func NewTextTransport(prefix string) *TextClientTransport {
	return &TextClientTransport{prefix: prefix}
}

func (t *TextClientTransport) RegisterToolProvider(ctx context.Context, prov server.Provider) ([]server.Tool, error) {
	// Typically no discovery needed for text transport
	return nil, nil
}

func (t *TextClientTransport) DeregisterToolProvider(ctx context.Context, prov server.Provider) error {
	return nil
}

func (t *TextClientTransport) CallTool(ctx context.Context, toolName string, args map[string]interface{}, prov server.Provider, l *string) (interface{}, error) {
	// For example, format output as simple text
	formatted := fmt.Sprintf("%s: called %s with args %v", t.prefix, toolName, args)
	return formatted, nil
}
